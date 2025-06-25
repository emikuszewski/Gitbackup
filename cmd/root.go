package cmd

import (
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/plainid/git-backup/config"
	"github.com/plainid/git-backup/plainid"
)

var (
	cfgFile        string
	cfg            *config.Config
	plainIDService *plainid.Service
	rootCmd        = &cobra.Command{
		Use:   "git-backup",
		Short: "Backup PlainID configuration to git repository",
		Long: `Git backup tool is used to backup PlainID configuration files to a git repository.
The main concept it's build around is versioning the configuration files, so you can easily
rollback to a previous version if needed.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.LoadConfig(cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to load configuration: %w", err)
			}

			plainIDService = plainid.NewService(*cfg)
			if err != nil {
				return fmt.Errorf("failed to create PlainID service: %w", err)
			}

			envs, err := plainIDService.Environments()
			if err != nil {
				return fmt.Errorf("failed to get environments for whildcard setup: %w", err)
			}

			var cfgEnvs []config.Environment
			// check for whildcard envs
			if cfg.PlainID.HasWildcardEnvironment() {
				for _, env := range envs {
					cfgEnvs = append(cfgEnvs, config.Environment{
						ID:   env.ID,
						Name: env.Name,
						Workspaces: []config.Workspace{ // if we have a wildcard environment, we assume all workspaces are included
							{ID: "*"}},
						Identities: []string{"*"},
					})
				}
			} else {
				for _, configEnv := range cfg.PlainID.Envs {
					for _, apiEnv := range envs {
						if configEnv.ID == apiEnv.ID {
							newEnv := configEnv
							newEnv.Name = apiEnv.Name
							cfgEnvs = append(cfgEnvs, newEnv)
							break
						}
					}
				}
			}

			// Process workspaces for each environment
			for i := range cfgEnvs {
				wss, err := plainIDService.Workspaces(cfgEnvs[i].ID)
				if err != nil {
					return fmt.Errorf("failed to get workspaces for environment %s: %w", cfgEnvs[i].ID, err)
				}

				var newWSs []config.Workspace
				if cfgEnvs[i].HasWildcardWorkspace() {
					for _, ws := range wss {
						newWSs = append(newWSs, config.Workspace{
							ID:   ws.ID,
							Name: ws.Name,
						})
					}
				} else {
					for _, configWs := range cfgEnvs[i].Workspaces {
						for _, apiWs := range wss {
							if configWs.ID == apiWs.ID {
								newWs := configWs
								newWs.Name = apiWs.Name
								newWSs = append(newWSs, newWs)
								break
							}
						}
					}
				}
				cfgEnvs[i].Workspaces = newWSs
			}

			// Process identities for each environment
			for i := range cfgEnvs {
				identities, err := plainIDService.Identities(cfgEnvs[i].ID)
				if err != nil {
					return fmt.Errorf("failed to get identities for environment %s: %w", cfgEnvs[i].ID, err)
				}

				var newIdentities []string
				if cfgEnvs[i].HasWildcardIdentities() {
					for _, identity := range identities {
						newIdentities = append(newIdentities, identity.TemplateID)
					}
					cfgEnvs[i].Identities = newIdentities
				}
			}

			cfg.PlainID.Envs = cfgEnvs
			return nil
		},
	}
)

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Failed to execute command")
		os.Exit(1)
	}
}

func init() {
	// Register all configuration flags
	config.RegisterFlags(rootCmd.PersistentFlags())

	// Add commands
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(restoreCmd)
	rootCmd.AddCommand(listCmd)
}
