package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/plainid/git-backup/repository"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup PlainID configuration to git",
	Long:  `Backup PlainID configuration to git and create a new tagged version.`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		log.Info().Msg("Executing backup command")
		if cfg.DryRun {
			log.Info().Msg("Dry run mode: will download configuration but won't push to git")
		}

		// Use the new helper functions for temp directory management
		tempDir, err := repository.CreateTempDir()
		if err != nil {
			return err
		}
		defer func() {
			if err == nil && cfg.Git.DeleteTempOnSuccess {
				repository.CleanupTempDir(tempDir)
			}

		}()

		log.Info().Msgf("Temporary directory created: %s", tempDir)

		repo, err := repository.CloneRemote(cfg.Git.Repo, cfg.Git.Branch, cfg.Git.Token, tempDir)
		if err != nil {
			return err
		}

		worktree, err := repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}

		// Process all environments and workspaces
		timestamp := time.Now().Format("20060102-150405")
		commitMsg := "Backup PlainID configuration for:"

		for _, env := range cfg.PlainID.Envs {
			envID := env.ID
			envName := env.Name
			log.Info().Msgf("Processing environment %s (%s) ...", envName, envID)
			envDir := fmt.Sprintf("%s/%s_%s", tempDir, envName, envID)

			// please create a directory if it doesn't exist
			if err = os.MkdirAll(envDir, 0755); err != nil {
				return fmt.Errorf("failed to create environment directory: %w", err)
			}

			// remove all files (identity files) from the directory first (except directories)
			err = removeFilesOnly(envDir)
			if err != nil {
				return fmt.Errorf("failed to remove files from env directory: %w", err)
			}

			err := fetchPlainIDEnvStuff(envDir, envID)
			if err != nil {
				return fmt.Errorf("failed to fetch PlainID Env configuration for env:%s: %w", envID, err)
			}

			for _, ws := range env.Workspaces {
				wsID := ws.ID     // unique
				wsName := ws.Name // unique and required

				log.Info().Msgf("Processing workspace %s (%s) ...", wsName, wsID)
				wsDir := fmt.Sprintf("%s/%s", envDir, wsName)
				// delete workspace content first
				err = os.RemoveAll(wsDir)
				if err != nil {
					return fmt.Errorf("failed to remove workspace directory: %w", err)
				}
				if err := os.MkdirAll(wsDir, 0755); err != nil {
					return fmt.Errorf("failed to create workspace directory: %w", err)
				}

				err := fetchPlainIDWSStuff(wsDir, envID, wsID)
				if err != nil {
					return fmt.Errorf("failed to fetch PlainID WS configuration for env:%s ws:%s: %w", envID, wsID, err)
				}
				// Add to commit message
				commitMsg += fmt.Sprintf(" env:%s ws:%s", envID, wsID)
			}
		}

		// Check for current HEAD reference
		_, err = repo.Head()
		isNewRepo := errors.Is(err, plumbing.ErrReferenceNotFound)

		// Instead of adding files one by one, use git's more comprehensive methods
		// that will handle both additions, modifications, and deletions
		worktree, err = repo.Worktree()
		if err != nil {
			return fmt.Errorf("failed to get worktree: %w", err)
		}

		// First add all files to the index - this will ensure any deleted files are tracked
		_, err = worktree.Add(".")
		if err != nil {
			return fmt.Errorf("failed to add all files to worktree: %w", err)
		}

		// Commit the changes
		commitHash, err := worktree.Commit(commitMsg, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "PlainID Git Backup",
				Email: "git-backup@plainid.com",
				When:  time.Now(),
			},
			AllowEmptyCommits: true, // Set the branch reference if this is a new repository
		})
		if err != nil {
			return fmt.Errorf("failed to commit changes: %w", err)
		}

		log.Info().Msgf("Changes committed: %s", commitHash.String())

		// For a new repository, create the branch reference
		if isNewRepo {
			// Create a reference for the branch
			branchRef := plumbing.NewHashReference(
				plumbing.NewBranchReferenceName(cfg.Git.Branch),
				commitHash,
			)

			// Set the reference in the repository
			if err := repo.Storer.SetReference(branchRef); err != nil {
				return fmt.Errorf("failed to set branch reference: %w", err)
			}
			log.Info().Msgf("Created branch: %s", cfg.Git.Branch)
		}

		_, err = repo.CreateTag(timestamp, commitHash, &git.CreateTagOptions{
			Tagger: &object.Signature{
				Name:  "PlainID Git Backup",
				Email: "git-backup@plainid.com",
				When:  time.Now(),
			},
			Message: fmt.Sprintf("Backup tag for %s", commitMsg),
		})
		if err != nil {
			return fmt.Errorf("failed to create tag: %w", err)
		}

		log.Info().Msgf("Created tag: %s", timestamp)

		// Skip pushing if dry run is enabled
		if cfg.DryRun {
			log.Info().Msg("Dry run mode: skipping push to remote repository")
			return nil
		}

		// Push changes to remote
		log.Info().Msg("Pushing changes to remote repository...")
		err = repo.Push(&git.PushOptions{
			Auth: &http.BasicAuth{
				Username: "oauth2",
				Password: cfg.Git.Token,
			},
			RefSpecs: []gitconfig.RefSpec{
				gitconfig.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", cfg.Git.Branch, cfg.Git.Branch)),
				gitconfig.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", timestamp, timestamp)),
			},
			Force: isNewRepo, // Force push for new repositories
		})
		if err != nil {
			return fmt.Errorf("failed to push changes: %w", err)
		}

		log.Info().Msg("Backup completed successfully")

		return nil
	},
}

func fetchPlainIDWSStuff(wsDir, envID, wsID string) error {
	apps, err := plainIDService.Applications(envID, wsID)
	if err != nil {
		return fmt.Errorf("failed to fetch apps: %w", err)
	}

	assetTemplatesIDs, err := plainIDService.AssetTemplateIDs(wsID)
	if err != nil {
		return fmt.Errorf("failed to fetch asset template IDs: %w", err)
	}

	for i, assetTemplateID := range assetTemplatesIDs {
		assetTemplate, err := plainIDService.AssetTemplate(envID, assetTemplateID)
		if err != nil {
			return fmt.Errorf("failed to fetch asset template %s : %w", assetTemplateID, err)
		}

		path := fmt.Sprintf("%s/asset-template_%d.json", wsDir, i)
		if err := os.WriteFile(path, []byte(assetTemplate), 0600); err != nil {
			return fmt.Errorf("failed to write asset template %s: %w", assetTemplateID, err)
		}
	}

	// Get the environment configuration
	env := cfg.PlainID.FindEnvironment(envID)
	if env == nil {
		return fmt.Errorf("environment %s not found in configuration", envID)
	}

	for _, app := range apps {
		appDir := fmt.Sprintf("%s/%s", wsDir, app.Name)
		if err := os.MkdirAll(appDir, 0755); err != nil {
			return fmt.Errorf("failed to create application directory: %w", err)
		}

		log.Info().Msgf("Processing application %s (%s) ...", app.Name, app.ID)

		path := fmt.Sprintf("%s/application.json", appDir)
		appJSON, err := app.AsJSON()
		if err != nil {
			return fmt.Errorf("failed to convert app to JSON: %w", err)
		}
		if err := os.WriteFile(path, []byte(appJSON), 0600); err != nil {
			return fmt.Errorf("failed to write app: %w", err)
		}

		policies, err := plainIDService.AppPolicies(envID, wsID, app.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch app policies: %w", err)
		}

		for i, policy := range policies {
			path := fmt.Sprintf("%s/policy_%d.srego", appDir, i)
			if err := os.WriteFile(path, []byte(policy), 0600); err != nil {
				return fmt.Errorf("failed to write policy: %w", err)
			}
		}

		apiMapperSet, err := plainIDService.AppAPIMapper(envID, app.ID)
		if err != nil {
			return fmt.Errorf("failed to fetch app api mapper: %w", err)
		}
		path = fmt.Sprintf("%s/api-mapper-set.json", appDir)
		if err := os.WriteFile(path, []byte(apiMapperSet), 0600); err != nil {
			return fmt.Errorf("failed to write policy: %w", err)
		}
	}
	return nil
}

func fetchPlainIDEnvStuff(envDir, envID string) error {
	// Get the environment configuration
	env := cfg.PlainID.FindEnvironment(envID)
	if env == nil {
		return fmt.Errorf("environment %s not found in configuration", envID)
	}

	log.Info().Msgf("Fetching PlainID environment configuration for %s ...", envID)

	// Process identity templates using identities from the environment config
	for _, identity := range env.Identities {
		identityTemplates, err := plainIDService.IdentityTemplates(envID, identity)
		if err != nil {
			return fmt.Errorf("failed to fetch app identity templates: %w", err)
		}
		path := fmt.Sprintf("%s/identity-template-%s.json", envDir, identity)
		if err := os.WriteFile(path, []byte(identityTemplates), 0600); err != nil {
			return fmt.Errorf("failed to write identity template: %w", err)
		}
	}

	// Fetch PAA groups
	paaGroups, err := plainIDService.PAAGroups(envID)
	if err != nil {
		return fmt.Errorf("failed to fetch PAA groups: %w", err)
	}

	for _, paaGroup := range paaGroups {
		log.Info().Msgf("Processing PAA group %s ...", paaGroup.ID)
		path := fmt.Sprintf("%s/paa-group_%s.json", envDir, paaGroup.ID)
		paaGroupJSON, err := paaGroup.ToJSON()
		if err != nil {
			return fmt.Errorf("failed to convert PAA group to JSON: %w", err)
		}

		if err := os.WriteFile(path, []byte(paaGroupJSON), 0600); err != nil {
			return fmt.Errorf("failed to write identity template: %w", err)
		}
	}

	return nil
}
func removeFilesOnly(dir string) error {
	log.Info().Msgf("cleaning up directory %s ...", dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		// If directory doesn't exist, no need to remove anything
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			// Recursively clean subdirectories
			if err := removeFilesOnly(path); err != nil {
				return err
			}
		} else {
			// Remove files
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}

	return nil
}
