package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	plumb "github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/plainid/git-backup/repository"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// listOptions holds command-specific options
type listOptions struct {
	envID string
	wsID  string
}

var listOpts listOptions

// tagInfo represents a filtered and parsed tag
type tagInfo struct {
	Name      string
	EnvID     string
	WsID      string
	Timestamp string
	Time      time.Time // For sorting
	Message   string    // Tag message
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent PlainID configuration backups",
	Long:  `List last 10 backups without restoring anything.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		log.Info().Msg("Executing list command")

		// Create temporary directory for git operations
		tempDir, err := repository.CreateTempDir()
		if err != nil {
			return err
		}
		defer repository.CleanupTempDir(tempDir)
		log.Info().Msgf("Temporary directory created: %s", tempDir)

		// Clone repository
		log.Info().Msg("Fetching repository information...")
		repo, err := repository.CloneRemote(cfg.Git.Repo, cfg.Git.Branch, cfg.Git.Token, tempDir)
		if err != nil {
			return fmt.Errorf("failed to access repository: %w", err)
		}

		// Fetch to ensure we have all tags
		err = repo.Fetch(&git.FetchOptions{
			Auth: &http.BasicAuth{
				Username: "oauth2",
				Password: cfg.Git.Token,
			},
			Tags: git.AllTags,
		})
		// Ignore "already up-to-date" errors
		if err != nil && err != git.NoErrAlreadyUpToDate {
			log.Warn().Msgf("Fetch warning: %v", err)
		}

		// Get all tags
		tagsIter, err := repo.Tags()
		if err != nil {
			return fmt.Errorf("failed to list tags: %w", err)
		}

		var filteredTags []tagInfo
		err = tagsIter.ForEach(func(ref *plumb.Reference) error {
			tagName := ref.Name().Short()

			// Only process tags that match the timestamp format (20060102-150405)
			parsedTime, err := time.Parse("20060102-150405", tagName)
			if err != nil {
				// Not a tag in our expected format, skip it
				return nil
			}

			// Get the tag object to read the message
			tagObj, err := repo.TagObject(ref.Hash())
			var message string
			if err == nil {
				message = tagObj.Message
			}

			// Apply env/ws filters if specified
			if listOpts.envID != "" && listOpts.wsID != "" {
				// Check if the message contains the specified env and ws
				envFilter := fmt.Sprintf("env:%s", listOpts.envID)
				wsFilter := fmt.Sprintf("ws:%s", listOpts.wsID)

				if !strings.Contains(message, envFilter) || !strings.Contains(message, wsFilter) {
					return nil
				}
			}

			// Parse env and ws IDs from message for display
			var envIDs, wsIDs []string
			msgParts := strings.Split(message, " ")
			for _, part := range msgParts {
				if strings.HasPrefix(part, "env:") {
					envIDs = append(envIDs, strings.TrimPrefix(part, "env:"))
				} else if strings.HasPrefix(part, "ws:") {
					wsIDs = append(wsIDs, strings.TrimPrefix(part, "ws:"))
				}
			}

			// Add tag to the filtered list
			filteredTags = append(filteredTags, tagInfo{
				Name:      tagName,
				Timestamp: tagName,
				Time:      parsedTime,
				Message:   message,
				EnvID:     strings.Join(envIDs, ","),
				WsID:      strings.Join(wsIDs, ","),
			})

			return nil
		})

		if err != nil {
			return fmt.Errorf("error processing tags: %w", err)
		}

		// Sort tags by timestamp (newest first)
		sort.Slice(filteredTags, func(i, j int) bool {
			return filteredTags[i].Time.After(filteredTags[j].Time)
		})

		// Display results
		if len(filteredTags) == 0 {
			if listOpts.envID != "" && listOpts.wsID != "" {
				fmt.Printf("No backups found for environment %s and workspace %s\n",
					listOpts.envID, listOpts.wsID)
			} else {
				fmt.Println("No backups found")
			}
			return nil
		}

		// Header message
		if listOpts.envID != "" && listOpts.wsID != "" {
			fmt.Printf("Recent backups for environment %s and workspace %s:\n\n",
				listOpts.envID, listOpts.wsID)
		} else {
			fmt.Println("Recent backups:")
		}

		// Display at most 10 most recent tags
		limit := 10
		if len(filteredTags) < limit {
			limit = len(filteredTags)
		}

		for i := 0; i < limit; i++ {
			tag := filteredTags[i]
			// Format timestamp for display if valid
			displayTime := tag.Timestamp
			if !tag.Time.IsZero() {
				displayTime = tag.Time.Format("2006-01-02 15:04:05")
			}

			if tag.EnvID != "" && tag.WsID != "" {
				fmt.Printf("%d. %s (env: %s, ws: %s, created: %s)\n",
					i+1, tag.Name, tag.EnvID, tag.WsID, displayTime)
			} else {
				fmt.Printf("%d. %s (created: %s)\n", i+1, tag.Name, displayTime)
			}
		}

		return nil
	},
}

func init() {
	// Add list-specific flags
	listCmd.Flags().StringVar(&listOpts.envID, "env-id", "", "Filter backups by environment ID")
	listCmd.Flags().StringVar(&listOpts.wsID, "ws-id", "", "Filter backups by workspace ID")
}
