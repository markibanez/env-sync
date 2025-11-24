package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func scanForEnvFiles(rootPath string) error {
	// Verify the path exists
	info, err := os.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", rootPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", rootPath)
	}

	var envFiles []string

	// Walk through the directory recursively
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip directories we can't access
			return nil
		}

		// Skip hidden directories and node_modules, vendor, etc.
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			if name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
		}

		// Check if it's a .env file
		if !info.IsDir() {
			name := info.Name()
			if name == ".env" || strings.HasPrefix(name, ".env.") {
				envFiles = append(envFiles, path)
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error scanning directory: %v", err)
	}

	if len(envFiles) == 0 {
		fmt.Println("No .env files found")
		return nil
	}

	// Save the found files
	if err := saveEnvFiles(envFiles); err != nil {
		return fmt.Errorf("error saving env files: %v", err)
	}

	fmt.Printf("Found and saved %d .env file(s):\n", len(envFiles))
	for _, file := range envFiles {
		fmt.Printf("  - %s\n", file)
	}

	return nil
}
