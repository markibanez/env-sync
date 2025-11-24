package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type EnvFileStore struct {
	Files []string `json:"files"`
}

func getStorageDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	storageDir := filepath.Join(homeDir, ".env-sync")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", err
	}

	return storageDir, nil
}

func getStorageFile() (string, error) {
	dir, err := getStorageDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "env-files.json"), nil
}

func saveEnvFiles(files []string) error {
	storageFile, err := getStorageFile()
	if err != nil {
		return err
	}

	store := EnvFileStore{
		Files: files,
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(storageFile, data, 0644)
}

func loadEnvFiles() ([]string, error) {
	storageFile, err := getStorageFile()
	if err != nil {
		return nil, err
	}

	// Check if file exists
	if _, err := os.Stat(storageFile); os.IsNotExist(err) {
		return []string{}, nil
	}

	data, err := os.ReadFile(storageFile)
	if err != nil {
		return nil, err
	}

	var store EnvFileStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}

	return store.Files, nil
}

func listEnvFiles() error {
	files, err := loadEnvFiles()
	if err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No .env files remembered. Run 'env-sync scan <path>' first.")
		return nil
	}

	fmt.Printf("Remembered %d .env file(s):\n", len(files))
	for i, file := range files {
		fmt.Printf("%d. %s\n", i+1, file)
	}

	return nil
}
