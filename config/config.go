// Package config loads and validates Soro's typed application configuration.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	Development = "development"
	Test        = "test"
	Production  = "production"
)

type Config struct {
	App      AppConfig      `yaml:"app"`
	Database DatabaseConfig `yaml:"database"`
}

type AppConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
}

type DatabaseConfig struct {
	URL              string        `yaml:"url"`
	MinConns         int32         `yaml:"min_conns"`
	MaxConns         int32         `yaml:"max_conns"`
	ConnectTimeout   time.Duration `yaml:"connect_timeout"`
	MaxConnLifetime  time.Duration `yaml:"max_conn_lifetime"`
	MaxConnIdleTime  time.Duration `yaml:"max_conn_idle_time"`
	HealthCheckEvery time.Duration `yaml:"health_check_period"`
}

func Defaults() Config {
	return Config{
		App: AppConfig{Name: "soro-app", Environment: Development},
		Database: DatabaseConfig{
			MinConns:         0,
			MaxConns:         10,
			ConnectTimeout:   5 * time.Second,
			MaxConnLifetime:  time.Hour,
			MaxConnIdleTime:  30 * time.Minute,
			HealthCheckEvery: time.Minute,
		},
	}
}

type loadOptions struct {
	directory string
	lookupEnv func(string) (string, bool)
}

type LoadOption func(*loadOptions)

func WithDirectory(directory string) LoadOption {
	return func(options *loadOptions) { options.directory = directory }
}

// WithEnvLookup supports deterministic tests and process supervisors that own
// their environment representation.
func WithEnvLookup(lookup func(string) (string, bool)) LoadOption {
	return func(options *loadOptions) { options.lookupEnv = lookup }
}

func Load(options ...LoadOption) (*Config, error) {
	settings := loadOptions{directory: "config", lookupEnv: os.LookupEnv}
	for _, option := range options {
		option(&settings)
	}

	result := Defaults()
	environment := envOr(settings.lookupEnv, "SORO_ENV", Development)
	result.App.Environment = environment

	for _, name := range []string{"application.yaml", environment + ".yaml"} {
		if err := mergeYAML(filepath.Join(settings.directory, name), &result); err != nil {
			return nil, err
		}
	}
	result.App.Environment = environment
	if err := applyEnvironment(&result, settings.lookupEnv); err != nil {
		return nil, err
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.App.Name) == "" {
		return errors.New("config: app.name is required")
	}
	switch c.App.Environment {
	case Development, Test, Production:
	default:
		return fmt.Errorf("config: unsupported SORO_ENV %q", c.App.Environment)
	}
	if c.App.Environment == Production && strings.TrimSpace(c.Database.URL) == "" {
		return errors.New("config: DATABASE_URL is required in production")
	}
	if c.Database.MinConns < 0 {
		return errors.New("config: database.min_conns cannot be negative")
	}
	if c.Database.MaxConns < 1 {
		return errors.New("config: database.max_conns must be at least 1")
	}
	if c.Database.MinConns > c.Database.MaxConns {
		return errors.New("config: database.min_conns cannot exceed max_conns")
	}
	if c.Database.ConnectTimeout <= 0 {
		return errors.New("config: database.connect_timeout must be positive")
	}
	return nil
}

func mergeYAML(path string, target *Config) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode config %s: %w", path, err)
	}
	return nil
}

func applyEnvironment(config *Config, lookup func(string) (string, bool)) error {
	if value, ok := lookup("DATABASE_URL"); ok {
		config.Database.URL = value
	}
	if value, ok := lookup("SORO_APP_NAME"); ok {
		config.App.Name = value
	}
	for name, destination := range map[string]*int32{
		"SORO_DATABASE_MIN_CONNS": &config.Database.MinConns,
		"SORO_DATABASE_MAX_CONNS": &config.Database.MaxConns,
	} {
		if value, ok := lookup(name); ok {
			parsed, err := strconv.ParseInt(value, 10, 32)
			if err != nil {
				return fmt.Errorf("config: %s must be an integer: %w", name, err)
			}
			*destination = int32(parsed)
		}
	}
	for name, destination := range map[string]*time.Duration{
		"SORO_DATABASE_CONNECT_TIMEOUT":     &config.Database.ConnectTimeout,
		"SORO_DATABASE_MAX_CONN_LIFETIME":   &config.Database.MaxConnLifetime,
		"SORO_DATABASE_MAX_CONN_IDLE_TIME":  &config.Database.MaxConnIdleTime,
		"SORO_DATABASE_HEALTH_CHECK_PERIOD": &config.Database.HealthCheckEvery,
	} {
		if value, ok := lookup(name); ok {
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("config: %s must be a duration: %w", name, err)
			}
			*destination = parsed
		}
	}
	return nil
}

func envOr(lookup func(string) (string, bool), key, fallback string) string {
	if value, ok := lookup(key); ok && value != "" {
		return value
	}
	return fallback
}
