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
	// Check if we need to migrate from old schema
	if err := db.migrateSchema(); err != nil {
		// Migration failed or not needed, continue with creation
	}

	// New schema using repo_id (git remote URL) instead of path
	query := `
	CREATE TABLE IF NOT EXISTS env_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		repo_id TEXT NOT NULL,
		relative_path TEXT NOT NULL,
		contents TEXT NOT NULL,
		file_hash TEXT NOT NULL,
		file_modified_at DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repo_id, relative_path)
	);
	`

	_, err := db.conn.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// Create index on repo_id for faster lookups
	indexQuery := `CREATE INDEX IF NOT EXISTS idx_env_files_repo_id ON env_files(repo_id);`
	_, err = db.conn.Exec(indexQuery)
	if err != nil {
		// Index might already exist, log but don't fail
		fmt.Printf("Note: index creation skipped (may already exist)\n")
	}

	return nil
}

// migrateSchema handles migration from old schema (path-based) to new schema (repo_id-based)
func (db *Database) migrateSchema() error {
	// Check if old table exists with 'path' column
	var tableName string
	err := db.conn.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='env_files'`).Scan(&tableName)
	if err != nil {
		// Table doesn't exist, no migration needed
		return nil
	}

	// Check if it has the old 'path' column
	rows, err := db.conn.Query(`PRAGMA table_info(env_files)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	hasPathColumn := false
	hasRepoIdColumn := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}
		if name == "path" {
			hasPathColumn = true
		}
		if name == "repo_id" {
			hasRepoIdColumn = true
		}
	}

	if hasRepoIdColumn {
		// Already migrated
		return nil
	}

	if hasPathColumn {
		// Need to migrate: drop old table (data will be lost, but it's encrypted with old paths anyway)
		fmt.Println("Migrating database schema to new git-based format...")
		fmt.Println("Note: Existing entries will be removed. Please re-sync after migration.")
		_, err := db.conn.Exec(`DROP TABLE env_files`)
		if err != nil {
			return fmt.Errorf("failed to drop old table: %v", err)
		}
	}

	return nil
}

// UpsertEnvFile inserts or updates an env file record
func (db *Database) UpsertEnvFile(repoID, relativePath, encryptedContents, fileHash, fileModTime string) error {
	// Use SQLite/LibSQL compatible upsert syntax
	query := `
	INSERT INTO env_files (repo_id, relative_path, contents, file_hash, file_modified_at, updated_at)
	VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT (repo_id, relative_path)
	DO UPDATE SET
		contents = excluded.contents,
		file_hash = excluded.file_hash,
		file_modified_at = excluded.file_modified_at,
		updated_at = CURRENT_TIMESTAMP
	`

	_, err := db.conn.Exec(query, repoID, relativePath, encryptedContents, fileHash, fileModTime)
	if err != nil {
		return fmt.Errorf("failed to upsert env file: %v", err)
	}

	return nil
}

// GetEnvFile retrieves an env file by repo_id and relative_path
func (db *Database) GetEnvFile(repoID, relativePath string) (string, error) {
	var contents string
	query := `SELECT contents FROM env_files WHERE repo_id = ? AND relative_path = ?`

	err := db.conn.QueryRow(query, repoID, relativePath).Scan(&contents)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("env file not found: %s:%s", repoID, relativePath)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query env file: %v", err)
	}

	return contents, nil
}

// GetEnvFileWithMetadata retrieves an env file with its metadata
func (db *Database) GetEnvFileWithMetadata(repoID, relativePath string) (*EnvFileRecord, error) {
	var record EnvFileRecord
	query := `SELECT repo_id, relative_path, contents, file_hash, file_modified_at, created_at, updated_at FROM env_files WHERE repo_id = ? AND relative_path = ?`

	err := db.conn.QueryRow(query, repoID, relativePath).Scan(&record.RepoID, &record.RelativePath, &record.Contents, &record.FileHash, &record.FileModifiedAt, &record.CreatedAt, &record.UpdatedAt)
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
	query := `SELECT repo_id, relative_path, file_hash, file_modified_at, created_at, updated_at FROM env_files ORDER BY repo_id, relative_path`

	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query env files: %v", err)
	}
	defer rows.Close()

	var records []EnvFileRecord
	for rows.Next() {
		var record EnvFileRecord
		if err := rows.Scan(&record.RepoID, &record.RelativePath, &record.FileHash, &record.FileModifiedAt, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}
		records = append(records, record)
	}

	return records, nil
}

type EnvFileRecord struct {
	RepoID         string
	RelativePath   string
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

		// Get git-based identifier or fallback to relative path
		repoID, relativePath, err := GetFileIdentifier(file, basePath)
		if err != nil {
			fmt.Printf("Warning: failed to get identifier for %s: %v\n", file, err)
			continue
		}

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
		if err := db.UpsertEnvFile(repoID, relativePath, encryptedContents, fileHash, fileModTime); err != nil {
			fmt.Printf("Warning: failed to upload %s: %v\n", file, err)
			continue
		}

		fmt.Printf("✓ Uploaded: %s → %s\n", relativePath, shortenRepoID(repoID))
	}

	return nil
}

// shortenRepoID returns a shortened version of repo ID for display
func shortenRepoID(repoID string) string {
	if repoID == "__local__" {
		return "[local]"
	}
	// Show just the repo name part (e.g., "github.com/user/repo" -> "user/repo")
	parts := strings.Split(repoID, "/")
	if len(parts) >= 3 {
		return strings.Join(parts[1:], "/")
	}
	return repoID
}
