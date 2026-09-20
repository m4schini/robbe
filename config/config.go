// SPDX-License-Identifier: TODO

// Package config holds viper configuration and default values
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	// developmentEnvVar toggles the InDevelopmentEnvironment config.
	developmentEnvVar = "DEVELOPMENT"
	// envPrefix is prepended to every environment variable robbe reads.
	envPrefix = "ROBBE"
	// DefaultGenerator is the path of the quadlet generator shipped with podman.
	DefaultGenerator = "/usr/lib/systemd/system-generators/podman-system-generator"
	// DefaultRef is the branch checked out when none is configured.
	DefaultRef = "main"
)

// Version is the build version, set by main from -ldflags.
var Version = "dev"

// ErrRepoURLMissing is returned by Load when no repository URL is configured.
var ErrRepoURLMissing = errors.New("repo.url is not configured")

// Auth mirrors ports.Auth so config stays free of application imports.
type Auth struct {
	SSHKey         string `mapstructure:"ssh_key"`
	SSHKeyPassword string `mapstructure:"ssh_key_password"`
	Token          string `mapstructure:"token"`
	Username       string `mapstructure:"username"`
}

// Repo describes the git repository holding the quadlet files.
type Repo struct {
	URL  string `mapstructure:"url"`
	Ref  string `mapstructure:"ref"`
	Auth Auth   `mapstructure:"auth"`
}

// Config is the fully resolved robbe configuration.
type Config struct {
	Repo Repo `mapstructure:"repo"`
	// Host selects hosts/<Host>/ in the repository. Defaults to os.Hostname().
	Host string `mapstructure:"host"`
	// Target is the directory the desired tree is written to.
	Target string `mapstructure:"target"`
	// Cache holds the git clone and the staging directory.
	Cache string `mapstructure:"cache"`
	// Generator is the path of podman-system-generator.
	Generator string `mapstructure:"generator"`
	// User selects the systemd user scope (systemctl --user, generator -user).
	User bool `mapstructure:"user"`
}

// InDevelopmentEnvironment reports whether the application is running in a InDevelopmentEnvironment environment.
// Set developmentEnvVar environment variable to truthy value such as "true" or "1" to enable this.
func InDevelopmentEnvironment() bool {
	value, ok := os.LookupEnv(developmentEnvVar)
	if !ok {
		return false
	}

	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false
	}

	return enabled
}

// Init configures viper: config file search paths, name, defaults and env lookup.
func Init() {
	// Find home directory.
	home, err := os.UserHomeDir()
	cobra.CheckErr(err)

	// Search config in home directory with name ".config" (without extension).
	viper.AddConfigPath(home)
	if runtime.GOOS == "linux" {
		viper.AddConfigPath("/etc/" + appNameLowercase)
	}
	viper.AddConfigPath(".")
	viper.SetConfigType("yaml")
	viper.SetConfigName("." + appNameLowercase)

	setDefaults(home)

	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv() // read in environment variables that match
	_ = viper.ReadInConfig()
}

// setDefaults registers every key with viper. Keys without a real default
// are registered as empty so that AutomaticEnv values reach Unmarshal.
func setDefaults(home string) {
	user := os.Geteuid() != 0

	viper.SetDefault("repo.url", "")
	viper.SetDefault("repo.ref", DefaultRef)
	viper.SetDefault("repo.auth.ssh_key", "")
	viper.SetDefault("repo.auth.ssh_key_password", "")
	viper.SetDefault("repo.auth.token", "")
	viper.SetDefault("repo.auth.username", "")
	viper.SetDefault("host", "")
	viper.SetDefault("target", defaultTarget(home, user))
	viper.SetDefault("cache", defaultCache(home, user))
	viper.SetDefault("generator", DefaultGenerator)
	viper.SetDefault("user", user)
}

func defaultTarget(home string, user bool) string {
	if !user {
		return "/etc/containers/systemd/" + appNameLowercase
	}

	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}

	return filepath.Join(base, "containers", "systemd", appNameLowercase)
}

func defaultCache(home string, user bool) string {
	if !user {
		return "/var/cache/" + appNameLowercase
	}

	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(home, ".cache")
	}

	return filepath.Join(base, appNameLowercase)
}

// Load unmarshals the viper state into a Config and validates it. Init must
// have run before.
func Load() (Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if cfg.Repo.URL == "" {
		return Config{}, ErrRepoURLMissing
	}

	if cfg.Host == "" {
		host, err := os.Hostname()
		if err != nil {
			return Config{}, fmt.Errorf("determine hostname: %w", err)
		}

		cfg.Host = host
	}

	return cfg, nil
}
