package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/plainid/git-backup/config"
	"github.com/plainid/git-backup/repository"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	restoreTag       string
	restoreTargetDir string
	restoreEnvID     string
	restoreWsID      string
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore PlainID configuration from git",
	Long:  `List recent backups and provide selection to restore from.`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Validate that tag and target-dir are only used together in non-interactive mode
		if restoreTag != "" {
			// Non-interactive mode - target-dir is required
			if restoreTargetDir == "" {
				return errors.New("target-dir is required when tag is specified (non-interactive mode)")
			}
		}

		// In dry-run mode, we need both tag and target-dir
		if cfg.DryRun && (restoreTag == "" || restoreTargetDir == "") {
			return errors.New("both tag and target-dir parameters are required when using dry-run with restore")
		}

		// Validate env-id and ws-id if they're provided
		if (restoreEnvID != "" && restoreWsID == "") || (restoreEnvID == "" && restoreWsID != "") {
			return errors.New("both env-id and ws-id must be provided together if one is specified")
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Info().Msg("Executing restore command")

		// Check if we're in non-interactive mode (tag is specified)
		if restoreTag != "" {
			log.Info().Str("tag", restoreTag).Msg("Non-interactive mode: Restoring from specific tag")
			log.Info().Str("targetDir", restoreTargetDir).Msg("Target directory specified")

			// Create target directory if it doesn't exist
			if err := os.MkdirAll(restoreTargetDir, 0755); err != nil {
				return fmt.Errorf("failed to create target directory: %w", err)
			}

			// Use temporary directory for git checkout
			tempDir, err := repository.CreateTempDir()
			if err != nil {
				return err
			}
			defer repository.CleanupTempDir(tempDir)

			log.Info().Str("tempDir", tempDir).Msg("Temporary directory created")

			// If env-id and ws-id are provided, only copy those specific directories
			if restoreEnvID != "" && restoreWsID != "" {
				log.Info().Str("envID", restoreEnvID).Str("wsID", restoreWsID).Msg("Filtering by environment and workspace")

				// Find the matching environment directory
				found := false

				entries, err := os.ReadDir(tempDir)
				if err != nil {
					return fmt.Errorf("failed to read temp directory: %w", err)
				}

				for _, entry := range entries {
					if entry.IsDir() && strings.HasSuffix(entry.Name(), "_"+restoreEnvID) {
						envDir := filepath.Join(tempDir, entry.Name())

						// Check for workspace within this environment
						wsEntries, err := os.ReadDir(envDir)
						if err != nil {
							return fmt.Errorf("failed to read environment directory: %w", err)
						}

						for _, wsEntry := range wsEntries {
							// Find matching workspace directory
							if wsEntry.IsDir() {
								wsDir := filepath.Join(envDir, wsEntry.Name())

								// Get workspace ID from configuration or use directory name
								// For now, we assume workspace directory name is the workspace name
								ws := findWorkspaceByNameOrID(restoreEnvID, restoreWsID, wsEntry.Name())
								if ws != nil {
									// Copy entire workspace directory to target
									log.Info().Str("source", wsDir).Str("target", restoreTargetDir).Msg("Copying workspace directory")

									if err := copyDir(wsDir, restoreTargetDir); err != nil {
										return fmt.Errorf("failed to copy workspace directory: %w", err)
									}

									found = true
									break
								}
							}
						}

						// Also copy environment-level files
						if found {
							// Copy environment-level files (templates, etc.)
							envFiles, err := os.ReadDir(envDir)
							if err != nil {
								return fmt.Errorf("failed to read environment directory files: %w", err)
							}

							for _, file := range envFiles {
								if !file.IsDir() {
									srcFile := filepath.Join(envDir, file.Name())
									dstFile := filepath.Join(restoreTargetDir, file.Name())
									log.Info().Str("source", srcFile).Str("target", dstFile).Msg("Copying environment file")

									if err := copyFile(srcFile, dstFile); err != nil {
										return fmt.Errorf("failed to copy environment file: %w", err)
									}
								}
							}

							break
						}
					}
				}

				if !found {
					return fmt.Errorf("could not find configuration for environment '%s' and workspace '%s' in tag '%s'", restoreEnvID, restoreWsID, restoreTag)
				}
			} else {
				// Copy everything from the tag to the target directory
				log.Info().Msg("No environment/workspace filter specified, copying all configuration")

				if err := copyDir(tempDir, restoreTargetDir); err != nil {
					return fmt.Errorf("failed to copy configuration: %w", err)
				}
			}

			if cfg.DryRun {
				log.Info().Msg("Dry run mode: Configuration has been checked out to target directory, but will not be processed further")
			} else {
				log.Info().Msg("Configuration has been checked out to target directory")
				// TODO: When restore to PlainID is implemented, add code here to upload the configuration
			}
		} else {
			log.Info().Msg("Interactive mode: Will present most recent backups for selection")
			// TODO: Implement interactive mode by showing recent tags and allowing selection
			// This can be implemented using the list command functionality
			return errors.New("interactive mode is not yet implemented")
		}

		log.Info().Msg("Restore operation completed")
		return nil
	},
}

// cloneAndCheckoutTag clones the repository and checks out the specified tag
func cloneAndCheckoutTag(tempDir, tag string) (*git.Repository, error) {
	log.Info().Str("repo", cfg.Git.Repo).Str("branch", cfg.Git.Branch).Msg("Cloning repository")

	// Clone the repository with default branch
	repo, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL: cfg.Git.Repo,
		Auth: &http.BasicAuth{
			Username: "oauth2",
			Password: cfg.Git.Token,
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to clone repository: %w", err)
	}

	// Get the worktree
	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	// Checkout the specified tag
	log.Info().Str("tag", tag).Msg("Checking out tag")
	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.ReferenceName(fmt.Sprintf("refs/tags/%s", tag)),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to checkout tag '%s': %w", tag, err)
	}

	return repo, nil
}

// findWorkspaceByNameOrID tries to find a workspace by its ID or name within the given environment
func findWorkspaceByNameOrID(envID, wsID, wsName string) *config.Workspace {
	// First try to find environment in configuration
	env := cfg.PlainID.FindEnvironment(envID)
	if env == nil {
		log.Warn().Str("envID", envID).Msg("Environment not found in configuration")
		return nil
	}

	// Check if workspace ID matches or if there's a wildcard
	for i, ws := range env.Workspaces {
		if ws.ID == wsID || ws.ID == "*" || (ws.Name == wsName && (wsID == "" || ws.ID == wsID)) {
			return &env.Workspaces[i]
		}
	}

	// If we have a wildcard environment and still haven't found a match, it's likely a configuration issue
	if env.HasWildcardWorkspace() {
		// Create a temporary workspace with the provided ID
		return &config.Workspace{ID: wsID, Name: wsName}
	}

	log.Warn().Str("envID", envID).Str("wsID", wsID).Str("wsName", wsName).Msg("Workspace not found in configuration")
	return nil
}

// copyFile copies a single file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create the destination directory if it doesn't exist
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Preserve file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}

// copyDir recursively copies a directory tree from src to dst
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func init() {
	// Add restore-specific flags
	restoreCmd.Flags().StringVar(&restoreTag, "tag", "", "Specific tag to restore from (for non-interactive mode)")
	restoreCmd.Flags().StringVar(&restoreTargetDir, "target-dir", "", "Target directory to check out configuration (for manual restoration)")
	restoreCmd.Flags().StringVar(&restoreEnvID, "env-id", "", "Environment ID to restore for (optional, for filtering)")
	restoreCmd.Flags().StringVar(&restoreWsID, "ws-id", "", "Workspace ID to restore for (optional, for filtering)")
}
