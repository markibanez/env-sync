package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func uploadEnvFiles(dbConnStr, password, basePath string) error {
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

	fmt.Printf("Uploading %d .env file(s)...\n", len(files))

	// Upload files
	if err := db.UploadEnvFiles(files, basePath, password); err != nil {
		return err
	}

	fmt.Println("\n✓ Upload complete!")
	return nil
}

func downloadEnvFiles(dbConnStr, password, outputPath string) error {
	// Connect to database
	db, err := NewDatabase(dbConnStr)
	if err != nil {
		return err
	}
	defer db.Close()

	// List all env files
	records, err := db.ListEnvFiles()
	if err != nil {
		return err
	}

	if len(records) == 0 {
		fmt.Println("No .env files found in database")
		return nil
	}

	fmt.Printf("Downloading %d .env file(s)...\n", len(records))

	for _, record := range records {
		// Get encrypted contents
		encryptedContents, err := db.GetEnvFile(record.Path, record.Filename)
		if err != nil {
			fmt.Printf("Warning: failed to get %s/%s: %v\n", record.Path, record.Filename, err)
			continue
		}

		// Decrypt contents
		contents, err := Decrypt(encryptedContents, password)
		if err != nil {
			fmt.Printf("Warning: failed to decrypt %s/%s: %v (wrong password?)\n", record.Path, record.Filename, err)
			continue
		}

		// Create output path
		// Convert Unix-style path back to OS-specific path
		relPath := strings.TrimPrefix(record.Path, "./")
		fullDir := filepath.Join(outputPath, filepath.FromSlash(relPath))

		// Create directory if it doesn't exist
		if err := os.MkdirAll(fullDir, 0755); err != nil {
			fmt.Printf("Warning: failed to create directory %s: %v\n", fullDir, err)
			continue
		}

		// Write file
		fullPath := filepath.Join(fullDir, record.Filename)
		if err := os.WriteFile(fullPath, []byte(contents), 0644); err != nil {
			fmt.Printf("Warning: failed to write %s: %v\n", fullPath, err)
			continue
		}

		fmt.Printf("✓ Downloaded: %s\n", fullPath)
	}

	fmt.Println("\n✓ Download complete!")
	return nil
}
