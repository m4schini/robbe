// SPDX-License-Identifier: TODO

// Package config holds viper configuration and default values
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/m4schini/robbe/internal/xdg"
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
	// configName is the config file name without extension, looked up in
	// every search directory as configName.yaml.
	configName = "config"
	// systemConfigRoot is the last search location, /etc/<app>, for hosts
	// that do not use XDG_CONFIG_DIRS.
	systemConfigRoot = "/etc"
)

// Version is the build version, set by main from -ldflags.
var Version = "dev"

// ConfigFile is the path given with --config. When set it replaces the
// search path and must exist.
var ConfigFile string

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
	// State holds the applied-commit marker.
	State string `mapstructure:"state"`
	// Runtime holds the lock file. Empty when XDG_RUNTIME_DIR is unset for
	// a non-root user; the caller then falls back to Cache.
	Runtime string `mapstructure:"runtime"`
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

// Init configures viper: config file search paths, name, defaults and env
// lookup, then reads the config file. A missing file in the search path is
// fine; a missing --config file or a malformed file is fatal.
func Init() {
	cobra.CheckErr(configure())
}

// configure is Init without the fatal exit, so tests can observe the error.
func configure() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home directory: %w", err)
	}

	user := os.Geteuid() != 0
	paths := xdg.Defaults(appNameLowercase, user, home)

	if ConfigFile != "" {
		viper.SetConfigFile(ConfigFile)
	} else {
		for _, dir := range searchDirs(home) {
			viper.AddConfigPath(dir)
		}

		viper.SetConfigName(configName)
	}

	viper.SetConfigType("yaml")

	setDefaults(paths, user)

	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv() // read in environment variables that match

	return readConfig()
}

// searchDirs lists the directories searched for config.yaml, in order:
// $XDG_CONFIG_HOME/<app>, <dir>/<app> for each $XDG_CONFIG_DIRS entry, and
// /etc/<app>.
func searchDirs(home string) []string {
	configDirs := xdg.ConfigDirs()

	roots := make([]string, 0, len(configDirs)+2) //nolint:mnd // ConfigHome in front, /etc at the end
	roots = append(roots, xdg.ConfigHome(home))
	roots = append(roots, configDirs...)
	roots = append(roots, systemConfigRoot)

	dirs := make([]string, 0, len(roots))
	for _, root := range roots {
		dirs = append(dirs, filepath.Join(root, appNameLowercase))
	}

	return dirs
}

// readConfig reads the config file. Not finding one in the search path is
// not an error; any other failure (missing --config file, malformed YAML)
// is reported so it cannot silently fall back to defaults.
func readConfig() error {
	err := viper.ReadInConfig()
	if err == nil {
		return nil
	}

	var notFound viper.ConfigFileNotFoundError
	if ConfigFile == "" && errors.As(err, &notFound) {
		return nil
	}

	name := viper.ConfigFileUsed()
	if name == "" {
		name = ConfigFile
	}

	return fmt.Errorf("read config %s: %w", name, err)
}

// setDefaults registers every key with viper. Keys without a real default
// are registered as empty so that AutomaticEnv values reach Unmarshal.
func setDefaults(paths xdg.Paths, user bool) {
	viper.SetDefault("repo.url", "")
	viper.SetDefault("repo.ref", DefaultRef)
	viper.SetDefault("repo.auth.ssh_key", "")
	viper.SetDefault("repo.auth.ssh_key_password", "")
	viper.SetDefault("repo.auth.token", "")
	viper.SetDefault("repo.auth.username", "")
	viper.SetDefault("host", "")
	viper.SetDefault("target", paths.Target)
	viper.SetDefault("cache", paths.Cache)
	viper.SetDefault("state", paths.State)
	// paths.Runtime is "" when XDG_RUNTIME_DIR is unset, which lets the
	// caller detect "not configured" and apply its fallback.
	viper.SetDefault("runtime", paths.Runtime)
	viper.SetDefault("generator", DefaultGenerator)
	viper.SetDefault("user", user)
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
