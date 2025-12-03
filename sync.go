package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SyncStats struct {
	FilesUploaded   int64
	FilesDownloaded int64
	FilesSkipped    int64
	FilesConflict   int64
}

type syncResult struct {
	file    string
	message string
	err     error
}

func syncEnvFiles(dbConnStr, password, basePath string, dryRun bool, numWorkers int) error {
	startTime := time.Now()

	// Load scanned env files
	files, err := loadEnvFiles()
	if err != nil {
		return fmt.Errorf("failed to load env files: %v", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no env files found. Run 'env-sync scan <path>' first")
	}

	// Connect to database
	dbStartTime := time.Now()
	db, err := NewDatabase(dbConnStr)
	if err != nil {
		return err
	}
	defer db.Close()
	dbConnectTime := time.Since(dbStartTime)

	// Initialize schema
	if err := db.InitSchema(); err != nil {
		return err
	}

	stats := &SyncStats{}

	if dryRun {
		fmt.Printf("DRY RUN MODE - No changes will be made\n")
	}
	fmt.Printf("Syncing %d .env file(s) with %d workers...\n\n", len(files), numWorkers)

	// Use worker pool for parallel processing
	if len(files) < numWorkers {
		numWorkers = len(files)
	}

	jobs := make(chan string, len(files))
	results := make(chan syncResult, len(files))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				msg, err := syncFileParallel(db, file, basePath, password, stats, dryRun)
				results <- syncResult{file: file, message: msg, err: err}
			}
		}()
	}

	// Send jobs
	syncStartTime := time.Now()
	for _, file := range files {
		jobs <- file
	}
	close(jobs)

	// Wait for workers in a goroutine
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	errCount := 0
	for result := range results {
		if result.err != nil {
			fmt.Printf("✗ Error syncing %s: %v\n", result.file, result.err)
			errCount++
		} else if result.message != "" {
			fmt.Println(result.message)
		}
	}
	syncTime := time.Since(syncStartTime)
	totalTime := time.Since(startTime)

	// Print summary
	fmt.Println("\n" + strings.Repeat("-", 50))
	if dryRun {
		fmt.Printf("Dry Run Summary (no changes made):\n")
	} else {
		fmt.Printf("Sync Summary:\n")
	}
	fmt.Printf("  ↑ Uploaded (local newer):   %d\n", atomic.LoadInt64(&stats.FilesUploaded))
	fmt.Printf("  ↓ Downloaded (remote newer): %d\n", atomic.LoadInt64(&stats.FilesDownloaded))
	fmt.Printf("  = Skipped (same):           %d\n", atomic.LoadInt64(&stats.FilesSkipped))
	if atomic.LoadInt64(&stats.FilesConflict) > 0 {
		fmt.Printf("  ⚠ Conflicts:                %d\n", atomic.LoadInt64(&stats.FilesConflict))
	}
	if errCount > 0 {
		fmt.Printf("  ✗ Errors:                   %d\n", errCount)
	}
	fmt.Println(strings.Repeat("-", 50))

	// Print performance metrics
	fmt.Printf("\nPerformance:\n")
	fmt.Printf("  Total files:      %d\n", len(files))
	fmt.Printf("  Workers used:     %d\n", numWorkers)
	fmt.Printf("  DB connect time:  %v\n", dbConnectTime.Round(time.Millisecond))
	fmt.Printf("  Sync time:        %v\n", syncTime.Round(time.Millisecond))
	fmt.Printf("  Total time:       %v\n", totalTime.Round(time.Millisecond))
	if syncTime.Seconds() > 0 {
		fmt.Printf("  Throughput:       %.1f files/sec\n", float64(len(files))/syncTime.Seconds())
	}

	return nil
}

// syncFileParallel is a parallel-safe version that returns a message instead of printing
func syncFileParallel(db *Database, filePath, basePath, password string, stats *SyncStats, dryRun bool) (string, error) {
	// Get git-based identifier or fallback to relative path
	repoID, relativePath, err := GetFileIdentifier(filePath, basePath)
	if err != nil {
		return "", fmt.Errorf("failed to get file identifier: %v", err)
	}

	displayName := fmt.Sprintf("%s (%s)", relativePath, shortenRepoID(repoID))

	// Get local file info
	localInfo, err := os.Stat(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to stat local file: %v", err)
	}
	localModTime := localInfo.ModTime().UTC()

	// Read local file contents for hash comparison
	localContents, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read local file: %v", err)
	}
	localHash := HashFile(string(localContents))

	// Check if file exists in database
	dbRecord, err := db.GetEnvFileWithMetadata(repoID, relativePath)
	if err != nil {
		return "", fmt.Errorf("failed to check database: %v", err)
	}

	if dbRecord == nil {
		// File doesn't exist in DB, upload it
		if !dryRun {
			if err := uploadFile(db, filePath, repoID, relativePath, password, localModTime, localHash); err != nil {
				return "", err
			}
		}
		atomic.AddInt64(&stats.FilesUploaded, 1)
		return fmt.Sprintf("↑ Uploaded: %s (new)%s", displayName, dryRunSuffix(dryRun)), nil
	}

	// Compare file hashes first (most reliable)
	if localHash == dbRecord.FileHash {
		// Files are identical, skip
		atomic.AddInt64(&stats.FilesSkipped, 1)
		return fmt.Sprintf("= Skipped: %s (identical)", displayName), nil
	}

	// Hashes differ, compare timestamps to determine direction
	// Parse database timestamp
	dbModTime, err := time.Parse("2006-01-02 15:04:05", dbRecord.FileModifiedAt)
	if err != nil {
		// Try RFC3339 format (ISO 8601) as fallback
		dbModTime, err = time.Parse(time.RFC3339, dbRecord.FileModifiedAt)
		if err != nil {
			return "", fmt.Errorf("failed to parse db timestamp: %v", err)
		}
	}

	// Compare timestamps (within 1 second tolerance for filesystem differences)
	timeDiff := localModTime.Sub(dbModTime).Seconds()

	if timeDiff > 1 {
		// Local file is newer, upload to database
		if !dryRun {
			if err := uploadFile(db, filePath, repoID, relativePath, password, localModTime, localHash); err != nil {
				return "", err
			}
		}
		atomic.AddInt64(&stats.FilesUploaded, 1)
		return fmt.Sprintf("↑ Uploaded: %s (local newer)%s", displayName, dryRunSuffix(dryRun)), nil
	} else if timeDiff < -1 {
		// Database file is newer, download from database
		if !dryRun {
			if err := downloadFile(db, dbRecord, filePath, password); err != nil {
				return "", err
			}
		}
		atomic.AddInt64(&stats.FilesDownloaded, 1)
		return fmt.Sprintf("↓ Downloaded: %s (remote newer)%s", displayName, dryRunSuffix(dryRun)), nil
	} else {
		// Timestamps are similar but hashes differ - this is a conflict
		// Default to uploading local (prefer local changes)
		if !dryRun {
			if err := uploadFile(db, filePath, repoID, relativePath, password, localModTime, localHash); err != nil {
				return "", err
			}
		}
		atomic.AddInt64(&stats.FilesUploaded, 1)
		return fmt.Sprintf("↑ Uploaded: %s (content changed, timestamps similar)%s", displayName, dryRunSuffix(dryRun)), nil
	}
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
