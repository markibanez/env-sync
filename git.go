package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// GitInfo holds git repository information for a file
type GitInfo struct {
	RemoteURL    string // Normalized remote URL (e.g., "github.com/user/repo")
	RelativePath string // Path relative to git root (e.g., "packages/api/.env")
	IsGitRepo    bool   // Whether the file is in a git repo
}

// GetGitInfo retrieves git information for a file path
func GetGitInfo(filePath string) (*GitInfo, error) {
	dir := filepath.Dir(filePath)

	// Find git root
	gitRoot, err := findGitRoot(dir)
	if err != nil {
		return &GitInfo{IsGitRepo: false}, nil
	}

	// Get remote URL
	remoteURL, err := getGitRemoteURL(gitRoot)
	if err != nil {
		return &GitInfo{IsGitRepo: false}, nil
	}

	// Normalize the remote URL
	normalizedURL := normalizeGitURL(remoteURL)

	// Get path relative to git root
	relPath, err := filepath.Rel(gitRoot, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get relative path: %v", err)
	}

	// Convert to Unix-style path for consistency
	relPath = filepath.ToSlash(relPath)

	return &GitInfo{
		RemoteURL:    normalizedURL,
		RelativePath: relPath,
		IsGitRepo:    true,
	}, nil
}

// findGitRoot finds the git repository root by looking for .git directory
func findGitRoot(startPath string) (string, error) {
	currentPath := startPath

	for {
		gitPath := filepath.Join(currentPath, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			if info.IsDir() {
				return currentPath, nil
			}
			// Could be a git worktree (file pointing to actual .git)
			if !info.IsDir() {
				return currentPath, nil
			}
		}

		// Move up one directory
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			// Reached root, no git repo found
			return "", fmt.Errorf("not a git repository")
		}
		currentPath = parentPath
	}
}

// getGitRemoteURL gets the origin remote URL using git command
func getGitRemoteURL(gitRoot string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = gitRoot

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get git remote: %v", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// normalizeGitURL normalizes various git URL formats to a consistent format
// Examples:
//   - git@github.com:user/repo.git -> github.com/user/repo
//   - https://github.com/user/repo.git -> github.com/user/repo
//   - https://github.com/user/repo -> github.com/user/repo
//   - ssh://git@github.com/user/repo.git -> github.com/user/repo
func normalizeGitURL(url string) string {
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Handle SSH format: git@github.com:user/repo
	sshRegex := regexp.MustCompile(`^git@([^:]+):(.+)$`)
	if matches := sshRegex.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// Handle SSH format: ssh://git@github.com/user/repo
	sshURLRegex := regexp.MustCompile(`^ssh://git@([^/]+)/(.+)$`)
	if matches := sshURLRegex.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// Handle HTTPS format: https://github.com/user/repo
	httpsRegex := regexp.MustCompile(`^https?://([^/]+)/(.+)$`)
	if matches := httpsRegex.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1] + "/" + matches[2]
	}

	// Fallback: return as-is
	return url
}

// GetFileIdentifier returns a unique identifier for a file
// Uses git remote + relative path for git repos, falls back to relative path from base
func GetFileIdentifier(filePath, basePath string) (repoID string, relativePath string, err error) {
	gitInfo, err := GetGitInfo(filePath)
	if err != nil {
		return "", "", err
	}

	if gitInfo.IsGitRepo && gitInfo.RemoteURL != "" {
		// Use git remote as repo identifier, relative path within repo
		return gitInfo.RemoteURL, gitInfo.RelativePath, nil
	}

	// Fallback: use relative path from base directory
	relPath, err := filepath.Rel(basePath, filePath)
	if err != nil {
		return "", "", fmt.Errorf("failed to get relative path: %v", err)
	}

	// Use a special prefix for non-git files
	return "__local__", filepath.ToSlash(relPath), nil
}
