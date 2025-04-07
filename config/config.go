package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// GitConfig holds the git-specific configuration
type GitConfig struct {
	Repo   string `mapstructure:"repo"`
	Token  string `mapstructure:"token"`
	Branch string `mapstructure:"branch"`
}

// Workspace represents a PlainID workspace
type Workspace struct {
	ID   string `mapstructure:"id"`
	Name string
}

// Environment represents a PlainID environment with its workspaces
type Environment struct {
	ID         string `mapstructure:"id"`
	Name       string
	Workspaces []Workspace `mapstructure:"workspaces"`
}

// HasWildcardWorkspace checks if the environment has a wildcard workspace configuration
func (e *Environment) HasWildcardWorkspace() bool {
	for _, workspace := range e.Workspaces {
		if workspace.ID == "*" {
			return true
		}
	}
	return false
}

// ContainsWorkspace checks if a specific workspace ID is included in this environment
// It returns true if the workspace ID matches or if there's a wildcard
func (e *Environment) ContainsWorkspace(workspaceID string) bool {
	for _, workspace := range e.Workspaces {
		if workspace.ID == workspaceID || workspace.ID == "*" {
			return true
		}
	}
	return false
}

// IsWildcard checks if this environment has a wildcard ID
func (e *Environment) IsWildcard() bool {
	return e.ID == "*"
}

// PlainIDConfig holds the PlainID-specific configuration
type PlainIDConfig struct {
	BaseURL      string        `mapstructure:"base-url"`
	ClientID     string        `mapstructure:"client-id"`
	ClientSecret string        `mapstructure:"client-secret"`
	Envs         []Environment `mapstructure:"envs"`
	Identities   []string      `mapstructure:"identities"`
}

// HasWildcardEnvironment checks if there's a wildcard environment in the configuration
func (p *PlainIDConfig) HasWildcardEnvironment() bool {
	for _, env := range p.Envs {
		if env.IsWildcard() {
			return true
		}
	}
	return false
}

// ContainsEnvironment checks if a specific environment ID is included in the configuration
// It returns true if the environment ID matches or if there's a wildcard environment
func (p *PlainIDConfig) ContainsEnvironment(envID string) bool {
	if p.HasWildcardEnvironment() {
		return true
	}

	for _, env := range p.Envs {
		if env.ID == envID {
			return true
		}
	}
	return false
}

// FindEnvironment returns the environment with the given ID, or nil if not found
func (p *PlainIDConfig) FindEnvironment(envID string) *Environment {
	// Check for exact match first
	for i, env := range p.Envs {
		if env.ID == envID {
			return &p.Envs[i]
		}
	}

	// If no exact match but we have a wildcard environment, return it
	if p.HasWildcardEnvironment() {
		for i, env := range p.Envs {
			if env.IsWildcard() {
				return &p.Envs[i]
			}
		}
	}

	return nil
}

// Config holds all the configuration parameters for the git-backup tool
type Config struct {
	// Structured configurations
	Git     GitConfig     `mapstructure:"git"`
	PlainID PlainIDConfig `mapstructure:"plainid"`

	// Command options
	DryRun bool `mapstructure:"dry-run"`
}

// LoadConfig loads the configuration from file, environment variables, and flags
func LoadConfig(flagSet *pflag.FlagSet) (*Config, error) {
	v := viper.New()

	// Set config name and paths
	v.SetConfigName(".git-backup")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath(homeDir())

	// Read config file if it exists
	if err := v.ReadInConfig(); err != nil {
		// It's okay if config file doesn't exist
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Bind environment variables
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	v.AutomaticEnv()

	// Bind command line flags if provided
	if flagSet != nil {
		if err := v.BindPFlags(flagSet); err != nil {
			return nil, fmt.Errorf("failed to bind flags: %w", err)
		}
	}

	// Unmarshal config
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate config
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// RegisterFlags registers all the configuration flags with the provided flag set
func RegisterFlags(flagSet *pflag.FlagSet) {
	// Git configuration
	flagSet.String("git.repo", "", "Git repository URL (git@ or https:// URL)")
	flagSet.String("git.token", "", "Git token for authentication")
	flagSet.String("git.branch", "main", "Git branch for storing configuration files")

	// PlainID configuration
	flagSet.String("plainid.base-url", "", "PlainID token endpoint URL")
	flagSet.String("plainid.client-id", "", "PlainID client ID")
	flagSet.String("plainid.client-secret", "", "PlainID client secret")

	// Global command options
	flagSet.Bool("dry-run", false, "Perform a dry run without making changes")
}

// validateConfig validates that all required configurations are present
func validateConfig(cfg *Config) error {
	var missingFields []string

	if cfg.Git.Repo == "" {
		missingFields = append(missingFields, "git.repo")
	}
	if cfg.Git.Token == "" {
		missingFields = append(missingFields, "git.token")
	}
	if cfg.Git.Branch == "" {
		missingFields = append(missingFields, "git.branch")
	}
	if cfg.PlainID.BaseURL == "" {
		missingFields = append(missingFields, "plainid.base-url")
	}
	if cfg.PlainID.ClientID == "" {
		missingFields = append(missingFields, "plainid.client-id")
	}
	if cfg.PlainID.ClientSecret == "" {
		missingFields = append(missingFields, "plainid.client-secret")
	}

	// Check for environments and workspaces
	if len(cfg.PlainID.Envs) == 0 {
		missingFields = append(missingFields, "plainid.envs")
	} else {
		// Check each environment has an ID and at least one workspace (unless it's a wildcard environment)
		for i, env := range cfg.PlainID.Envs {
			if env.ID == "" {
				missingFields = append(missingFields, fmt.Sprintf("plainid.envs[%d].id", i))
			}
			// For wildcard environments, workspaces are optional
			if len(env.Workspaces) == 0 && !env.IsWildcard() {
				missingFields = append(missingFields, fmt.Sprintf("plainid.envs[%d].workspaces", i))
			}
		}
	}

	// Check for identities
	if len(cfg.PlainID.Identities) == 0 {
		missingFields = append(missingFields, "plainid.identities")
	}

	if len(missingFields) > 0 {
		return errors.New("missing required configuration: " + strings.Join(missingFields, ", "))
	}

	return nil
}

// homeDir returns the user's home directory or current directory if it can't be determined
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

// DefaultConfigFilePath returns the default path for the config file
func DefaultConfigFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".git-backup"
	}
	return filepath.Join(home, ".git-backup")
}
