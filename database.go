package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

type Database struct {
	conn *sql.DB
}

// NewDatabase creates a new database connection
// Supports both LibSQL (Turso) and PostgreSQL
// LibSQL URL format: libsql://[host]?authToken=[token]
// PostgreSQL URL format: postgres://user:pass@host:port/dbname
func NewDatabase(connString string) (*Database, error) {
	var driver string

	// Detect database type from connection string
	if strings.HasPrefix(connString, "libsql://") || strings.HasPrefix(connString, "http://") || strings.HasPrefix(connString, "https://") {
		driver = "libsql"
	} else if strings.HasPrefix(connString, "postgres://") || strings.HasPrefix(connString, "postgresql://") {
		driver = "postgres"
	} else {
		return nil, fmt.Errorf("unsupported database URL format. Use 'libsql://' for Turso or 'postgres://' for PostgreSQL")
	}

	db, err := sql.Open(driver, connString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	return &Database{conn: db}, nil
}

// Close closes the database connection
func (db *Database) Close() error {
	return db.conn.Close()
}

// InitSchema creates the env_files table if it doesn't exist
func (db *Database) InitSchema() error {
	// SQLite/LibSQL compatible schema (works with both LibSQL and PostgreSQL)
	query := `
	CREATE TABLE IF NOT EXISTS env_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT NOT NULL,
		filename TEXT NOT NULL,
		contents TEXT NOT NULL,
		file_hash TEXT NOT NULL,
		file_modified_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(path, filename)
	);
	`

	_, err := db.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// Create index separately (some databases don't support IF NOT EXISTS on indexes in CREATE TABLE)
	indexQuery := `CREATE INDEX IF NOT EXISTS idx_env_files_path ON env_files(path);`
	_, err = db.conn.Exec(indexQuery)
	if err != nil {
		// Index might already exist, log but don't fail
		fmt.Printf("Note: index creation skipped (may already exist)\n")
	}

	return nil
}

// UpsertEnvFile inserts or updates an env file record
func (db *Database) UpsertEnvFile(path, filename, encryptedContents, fileHash, fileModTime string) error {
	// Use SQLite/LibSQL compatible upsert syntax
	query := `
	INSERT INTO env_files (path, filename, contents, file_hash, file_modified_at, updated_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT (path, filename)
	DO UPDATE SET
		contents = excluded.contents,
		file_hash = excluded.file_hash,
		file_modified_at = excluded.file_modified_at,
		updated_at = CURRENT_TIMESTAMP
	`

	_, err := db.conn.Exec(query, path, filename, encryptedContents, fileHash, fileModTime)
	if err != nil {
		return fmt.Errorf("failed to upsert env file: %v", err)
	}

	return nil
}

// GetEnvFile retrieves an env file by path and filename
func (db *Database) GetEnvFile(path, filename string) (string, error) {
	var contents string
	query := `SELECT contents FROM env_files WHERE path = ? AND filename = ?`

	err := db.conn.QueryRow(query, path, filename).Scan(&contents)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("env file not found: %s/%s", path, filename)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query env file: %v", err)
	}

	return contents, nil
}

// GetEnvFileWithMetadata retrieves an env file with its metadata
func (db *Database) GetEnvFileWithMetadata(path, filename string) (*EnvFileRecord, error) {
	var record EnvFileRecord
	query := `SELECT path, filename, contents, file_hash, file_modified_at, created_at, updated_at FROM env_files WHERE path = ? AND filename = ?`

	err := db.conn.QueryRow(query, path, filename).Scan(&record.Path, &record.Filename, &record.Contents, &record.FileHash, &record.FileModifiedAt, &record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil // Not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query env file: %v", err)
	}

	return &record, nil
}

// ListEnvFiles returns all env files in the database
func (db *Database) ListEnvFiles() ([]EnvFileRecord, error) {
	query := `SELECT path, filename, file_hash, file_modified_at, created_at, updated_at FROM env_files ORDER BY path, filename`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query env files: %v", err)
	}
	defer rows.Close()

	var records []EnvFileRecord
	for rows.Next() {
		var record EnvFileRecord
		if err := rows.Scan(&record.Path, &record.Filename, &record.FileHash, &record.FileModifiedAt, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}
		records = append(records, record)
	}

	return records, nil
}

type EnvFileRecord struct {
	Path           string
	Filename       string
	Contents       string
	FileHash       string
	FileModifiedAt string
	CreatedAt      string
	UpdatedAt      string
}

// toUnixRelativePath converts an absolute path to a Unix-style relative path
func toUnixRelativePath(absolutePath, basePath string) (string, error) {
	relPath, err := filepath.Rel(basePath, absolutePath)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path: %v", err)
	}

	// Convert to Unix-style path
	unixPath := filepath.ToSlash(relPath)

	// Ensure it starts with ./
	if !strings.HasPrefix(unixPath, "./") && !strings.HasPrefix(unixPath, "../") {
		unixPath = "./" + unixPath
	}

	return unixPath, nil
}

// getFileDirectory returns the directory part of the path in Unix style
func getFileDirectory(filePath, basePath string) (string, error) {
	dir := filepath.Dir(filePath)
	return toUnixRelativePath(dir, basePath)
}

// UploadEnvFiles uploads env files to the database with encryption
func (db *Database) UploadEnvFiles(files []string, basePath, password string) error {
	for _, file := range files {
		// Read file contents
		contents, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("Warning: failed to read %s: %v\n", file, err)
			continue
		}

		// Encrypt contents
		encryptedContents, err := Encrypt(string(contents), password)
		if err != nil {
			fmt.Printf("Warning: failed to encrypt %s: %v\n", file, err)
			continue
		}

		// Get relative path
		relDir, err := getFileDirectory(file, basePath)
		if err != nil {
			fmt.Printf("Warning: failed to get relative path for %s: %v\n", file, err)
			continue
		}

		filename := filepath.Base(file)

		// Get file modification time
		fileInfo, err := os.Stat(file)
		if err != nil {
			fmt.Printf("Warning: failed to stat %s: %v\n", file, err)
			continue
		}
		fileModTime := fileInfo.ModTime().UTC().Format("2006-01-02 15:04:05")

		// Calculate file hash
		fileHash := HashFile(string(contents))

		// Upload to database
		if err := db.UpsertEnvFile(relDir, filename, encryptedContents, fileHash, fileModTime); err != nil {
			fmt.Printf("Warning: failed to upload %s: %v\n", file, err)
			continue
		}

		fmt.Printf("✓ Uploaded: %s/%s\n", relDir, filename)
	}

	return nil
}
