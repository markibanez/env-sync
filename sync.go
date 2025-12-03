package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type SyncStats struct {
	FilesUploaded   int
	FilesDownloaded int
	FilesSkipped    int
	FilesConflict   int
}

func syncEnvFiles(dbConnStr, password, basePath string, dryRun bool) error {
	// Load scanned env files
	files, err := loadEnvFiles()
	if err != nil {
		return fmt.Errorf("failed to load env files: %v", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no env files found. Run 'env-sync scan <path>' first")
	}

	// Connect to database
	db, err := NewDatabase(dbConnStr)
	if err != nil {
		return err
	}
	defer db.Close()

	// Initialize schema
	if err := db.InitSchema(); err != nil {
		return err
	}

	stats := &SyncStats{}

	if dryRun {
		fmt.Printf("DRY RUN MODE - No changes will be made\n")
	}
	fmt.Printf("Syncing %d .env file(s)...\n\n", len(files))

	// Process each local file
	for _, file := range files {
		if err := syncFile(db, file, basePath, password, stats, dryRun); err != nil {
			fmt.Printf("✗ Error syncing %s: %v\n", file, err)
			continue
		}
	}

	// Print summary
	fmt.Println("\n" + strings.Repeat("-", 50))
	if dryRun {
		fmt.Printf("Dry Run Summary (no changes made):\n")
	} else {
		fmt.Printf("Sync Summary:\n")
	}
	fmt.Printf("  ↑ Uploaded (local newer):   %d\n", stats.FilesUploaded)
	fmt.Printf("  ↓ Downloaded (remote newer): %d\n", stats.FilesDownloaded)
	fmt.Printf("  = Skipped (same):           %d\n", stats.FilesSkipped)
	if stats.FilesConflict > 0 {
		fmt.Printf("  ⚠ Conflicts:                %d\n", stats.FilesConflict)
	}
	fmt.Println(strings.Repeat("-", 50))

	return nil
}

func syncFile(db *Database, filePath, basePath, password string, stats *SyncStats, dryRun bool) error {
	// Get git-based identifier or fallback to relative path
	repoID, relativePath, err := GetFileIdentifier(filePath, basePath)
	if err != nil {
		return fmt.Errorf("failed to get file identifier: %v", err)
	}

	displayName := fmt.Sprintf("%s (%s)", relativePath, shortenRepoID(repoID))

	// Get local file info
	localInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %v", err)
	}
	localModTime := localInfo.ModTime().UTC()

	// Read local file contents for hash comparison
	localContents, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %v", err)
	}
	localHash := HashFile(string(localContents))

	// Check if file exists in database
	dbRecord, err := db.GetEnvFileWithMetadata(repoID, relativePath)
	if err != nil {
		return fmt.Errorf("failed to check database: %v", err)
	}

	if dbRecord == nil {
		// File doesn't exist in DB, upload it
		if !dryRun {
			if err := uploadFile(db, filePath, repoID, relativePath, password, localModTime, localHash); err != nil {
				return err
			}
		}
		fmt.Printf("↑ Uploaded: %s (new)%s\n", displayName, dryRunSuffix(dryRun))
		stats.FilesUploaded++
		return nil
	}

	// Compare file hashes first (most reliable)
	if localHash == dbRecord.FileHash {
		// Files are identical, skip
		fmt.Printf("= Skipped: %s (identical)\n", displayName)
		stats.FilesSkipped++
		return nil
	}

	// Hashes differ, compare timestamps to determine direction
	// Parse database timestamp
	dbModTime, err := time.Parse("2006-01-02 15:04:05", dbRecord.FileModifiedAt)
	if err != nil {
		// Try RFC3339 format (ISO 8601) as fallback
		dbModTime, err = time.Parse(time.RFC3339, dbRecord.FileModifiedAt)
		if err != nil {
			return fmt.Errorf("failed to parse db timestamp: %v", err)
		}
	}

	// Compare timestamps (within 1 second tolerance for filesystem differences)
	timeDiff := localModTime.Sub(dbModTime).Seconds()

	if timeDiff > 1 {
		// Local file is newer, upload to database
		if !dryRun {
			if err := uploadFile(db, filePath, repoID, relativePath, password, localModTime, localHash); err != nil {
				return err
			}
		}
		fmt.Printf("↑ Uploaded: %s (local newer)%s\n", displayName, dryRunSuffix(dryRun))
		stats.FilesUploaded++
	} else if timeDiff < -1 {
		// Database file is newer, download from database
		if !dryRun {
			if err := downloadFile(db, dbRecord, filePath, password); err != nil {
				return err
			}
		}
		fmt.Printf("↓ Downloaded: %s (remote newer)%s\n", displayName, dryRunSuffix(dryRun))
		stats.FilesDownloaded++
	} else {
		// Timestamps are similar but hashes differ - this is a conflict
		// Default to uploading local (prefer local changes)
		if !dryRun {
			if err := uploadFile(db, filePath, repoID, relativePath, password, localModTime, localHash); err != nil {
				return err
			}
		}
		fmt.Printf("↑ Uploaded: %s (content changed, timestamps similar)%s\n", displayName, dryRunSuffix(dryRun))
		stats.FilesUploaded++
	}

	return nil
}

func dryRunSuffix(dryRun bool) string {
	if dryRun {
		return " [DRY RUN]"
	}
	return ""
}

func uploadFile(db *Database, filePath, repoID, relativePath, password string, modTime time.Time, fileHash string) error {
	// Read file contents
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// Encrypt contents
	encryptedContents, err := Encrypt(string(contents), password)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %v", err)
	}

	// Format mod time
	fileModTime := modTime.Format("2006-01-02 15:04:05")

	// Upload to database
	if err := db.UpsertEnvFile(repoID, relativePath, encryptedContents, fileHash, fileModTime); err != nil {
		return fmt.Errorf("failed to upload: %v", err)
	}

	return nil
}

func downloadFile(db *Database, record *EnvFileRecord, localPath, password string) error {
	// Decrypt contents
	contents, err := Decrypt(record.Contents, password)
	if err != nil {
		return fmt.Errorf("failed to decrypt: %v (wrong password?)", err)
	}

	// Parse the database timestamp - try multiple formats
	var dbModTime time.Time
	dbModTime, err = time.Parse("2006-01-02 15:04:05", record.FileModifiedAt)
	if err != nil {
		// Try RFC3339 format (ISO 8601)
		dbModTime, err = time.Parse(time.RFC3339, record.FileModifiedAt)
		if err != nil {
			return fmt.Errorf("failed to parse timestamp: %v", err)
		}
	}

	// Write file
	if err := os.WriteFile(localPath, []byte(contents), 0644); err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	// Set file modification time to match database
	if err := os.Chtimes(localPath, dbModTime, dbModTime); err != nil {
		// Non-critical error, just log it
		fmt.Printf("  (note: couldn't set file time: %v)\n", err)
	}

	return nil
}
