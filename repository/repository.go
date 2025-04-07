package repository

import (
	"errors"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http" // For HTTPS authentication
	"github.com/rs/zerolog/log"
)

// CreateTempDir creates a temporary directory for git operations
func CreateTempDir() (string, error) {
	tempDir, err := os.MkdirTemp("", "git-backup-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}
	return tempDir, nil
}

// CleanupTempDir removes the temporary directory
func CleanupTempDir(path string) {
	if path != "" {
		if err := os.RemoveAll(path); err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Failed to clean up temporary directory")
		}
	}
}

func CloneRemote(remoteURL, branchName, token, localPath string) (*git.Repository, error) {
	// First, check if repository already exists locally
	repo, err := git.PlainOpen(localPath)
	if err == nil {
		log.Info().Msg("Repository already exists locally, using it")
		return repo, nil
	}

	// Try to clone the repository
	repo, err = git.PlainClone(localPath, false, &git.CloneOptions{
		URL:           remoteURL,
		SingleBranch:  true,
		Depth:         1,
		ReferenceName: plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s", branchName)),
		Progress:      log.Logger,
		Auth: &http.BasicAuth{
			Username: "oauth2",
			Password: token,
		},
	})

	if err != nil {
		// If it's an empty repository or reference not found,
		// initialize a new repository and set up remote
		if errors.Is(err, transport.ErrEmptyRemoteRepository) ||
			errors.Is(err, plumbing.ErrReferenceNotFound) {
			log.Info().Msg("Empty or new repository, initializing it")
			return initializeRepository(remoteURL, branchName, token, localPath)
		}
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	return repo, nil
}

// initializeRepository creates a new local repository and sets up the remote
func initializeRepository(remoteURL, branchName, token, localPath string) (*git.Repository, error) {
	// Make sure directory exists
	if err := os.MkdirAll(localPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Initialize a new repository
	repo, err := git.PlainInit(localPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository: %w", err)
	}

	_, err = repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{remoteURL},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create remote: %w", err)
	}

	log.Info().Msgf("Initialized new repository with remote: %s", remoteURL)
	return repo, nil
}
