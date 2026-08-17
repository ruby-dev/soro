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
	App           AppConfig           `yaml:"app"`
	HTTP          HTTPConfig          `yaml:"http"`
	Database      DatabaseConfig      `yaml:"database"`
	Jobs          JobsConfig          `yaml:"jobs"`
	Mail          MailConfig          `yaml:"mail"`
	Observability ObservabilityConfig `yaml:"observability"`
}

type AppConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
}

type HTTPConfig struct {
	Port              int           `yaml:"port"`
	APIBasePath       string        `yaml:"api_base_path"`
	MaxRequestBody    int64         `yaml:"max_request_body"`
	ReadHeaderTimeout time.Duration `yaml:"read_header_timeout"`
	ReadTimeout       time.Duration `yaml:"read_timeout"`
	WriteTimeout      time.Duration `yaml:"write_timeout"`
	IdleTimeout       time.Duration `yaml:"idle_timeout"`
	ShutdownTimeout   time.Duration `yaml:"shutdown_timeout"`
	ReadinessTimeout  time.Duration `yaml:"readiness_timeout"`
}

type JobsConfig struct {
	Enabled         bool          `yaml:"enabled"`
	DefaultQueue    string        `yaml:"default_queue"`
	Workers         int           `yaml:"workers"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type MailConfig struct {
	Transport string     `yaml:"transport"`
	From      string     `yaml:"from"`
	Queue     string     `yaml:"queue"`
	SMTP      SMTPConfig `yaml:"smtp"`
}

type SMTPConfig struct {
	Host               string        `yaml:"host"`
	Port               int           `yaml:"port"`
	Username           string        `yaml:"username"`
	Password           string        `yaml:"password"`
	StartTLS           bool          `yaml:"starttls"`
	ImplicitTLS        bool          `yaml:"implicit_tls"`
	InsecureSkipVerify bool          `yaml:"insecure_skip_verify"`
	Timeout            time.Duration `yaml:"timeout"`
}

type ObservabilityConfig struct {
	Enabled      bool          `yaml:"enabled"`
	OTLPEndpoint string        `yaml:"otlp_endpoint"`
	OTLPTimeout  time.Duration `yaml:"otlp_timeout"`
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
		HTTP: HTTPConfig{
			Port: 8080, APIBasePath: "/api", MaxRequestBody: 1024 * 1024,
			ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second,
			IdleTimeout: 60 * time.Second, ShutdownTimeout: 30 * time.Second, ReadinessTimeout: 2 * time.Second,
		},
		Jobs:          JobsConfig{DefaultQueue: "default", Workers: 10, ShutdownTimeout: 30 * time.Second},
		Mail:          MailConfig{Transport: "console", From: "noreply@example.com", Queue: "mailers", SMTP: SMTPConfig{Port: 587, StartTLS: true, Timeout: 10 * time.Second}},
		Observability: ObservabilityConfig{Enabled: true, OTLPTimeout: 10 * time.Second},
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
	if c.HTTP.Port < 1 || c.HTTP.Port > 65535 {
		return errors.New("config: http.port must be between 1 and 65535")
	}
	if !strings.HasPrefix(c.HTTP.APIBasePath, "/") || (len(c.HTTP.APIBasePath) > 1 && strings.HasSuffix(c.HTTP.APIBasePath, "/")) {
		return errors.New("config: http.api_base_path must begin with / and must not end with /")
	}
	if c.HTTP.MaxRequestBody < 1 {
		return errors.New("config: http.max_request_body must be positive")
	}
	for name, value := range map[string]time.Duration{
		"read_header_timeout": c.HTTP.ReadHeaderTimeout, "read_timeout": c.HTTP.ReadTimeout,
		"write_timeout": c.HTTP.WriteTimeout, "idle_timeout": c.HTTP.IdleTimeout,
		"shutdown_timeout": c.HTTP.ShutdownTimeout, "readiness_timeout": c.HTTP.ReadinessTimeout,
	} {
		if value <= 0 {
			return fmt.Errorf("config: http.%s must be positive", name)
		}
	}
	if strings.TrimSpace(c.Jobs.DefaultQueue) == "" || c.Jobs.Workers < 1 || c.Jobs.ShutdownTimeout <= 0 {
		return errors.New("config: jobs default_queue, workers, and shutdown_timeout are invalid")
	}
	if c.Mail.Transport != "console" && c.Mail.Transport != "smtp" && c.Mail.Transport != "capture" {
		return fmt.Errorf("config: unsupported mail transport %q", c.Mail.Transport)
	}
	if strings.TrimSpace(c.Mail.From) == "" || strings.TrimSpace(c.Mail.Queue) == "" {
		return errors.New("config: mail.from and mail.queue are required")
	}
	if c.Mail.Transport == "smtp" {
		if c.Mail.SMTP.Host == "" || c.Mail.SMTP.Port < 1 || c.Mail.SMTP.Port > 65535 || c.Mail.SMTP.Timeout <= 0 {
			return errors.New("config: valid SMTP host, port, and timeout are required")
		}
		if c.Mail.SMTP.StartTLS && c.Mail.SMTP.ImplicitTLS {
			return errors.New("config: SMTP STARTTLS and implicit TLS are mutually exclusive")
		}
		if (c.Mail.SMTP.Username == "") != (c.Mail.SMTP.Password == "") {
			return errors.New("config: SMTP username and password must be configured together")
		}
	}
	if c.App.Environment == Production && c.Mail.Transport != "smtp" {
		return errors.New("config: production mail transport must be smtp")
	}
	if c.App.Environment == Production && (!c.Mail.SMTP.StartTLS && !c.Mail.SMTP.ImplicitTLS || c.Mail.SMTP.InsecureSkipVerify) {
		return errors.New("config: production SMTP requires verified TLS")
	}
	if c.Observability.OTLPTimeout <= 0 {
		return errors.New("config: observability.otlp_timeout must be positive")
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
	if value, ok := lookup("SORO_HTTP_PORT"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("config: SORO_HTTP_PORT must be an integer: %w", err)
		}
		config.HTTP.Port = parsed
	}
	if value, ok := lookup("SORO_API_BASE_PATH"); ok {
		config.HTTP.APIBasePath = value
	}
	if value, ok := lookup("SORO_MAX_REQUEST_BODY"); ok {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("config: SORO_MAX_REQUEST_BODY must be an integer: %w", err)
		}
		config.HTTP.MaxRequestBody = parsed
	}
	for name, destination := range map[string]*string{
		"SORO_JOBS_DEFAULT_QUEUE":     &config.Jobs.DefaultQueue,
		"SORO_MAIL_TRANSPORT":         &config.Mail.Transport,
		"SORO_MAIL_FROM":              &config.Mail.From,
		"SORO_MAIL_QUEUE":             &config.Mail.Queue,
		"SMTP_HOST":                   &config.Mail.SMTP.Host,
		"SMTP_USERNAME":               &config.Mail.SMTP.Username,
		"SMTP_PASSWORD":               &config.Mail.SMTP.Password,
		"OTEL_EXPORTER_OTLP_ENDPOINT": &config.Observability.OTLPEndpoint,
	} {
		if value, ok := lookup(name); ok {
			*destination = value
		}
	}
	for name, destination := range map[string]*bool{
		"SORO_JOBS_ENABLED":              &config.Jobs.Enabled,
		"SORO_OTEL_ENABLED":              &config.Observability.Enabled,
		"SORO_SMTP_STARTTLS":             &config.Mail.SMTP.StartTLS,
		"SORO_SMTP_IMPLICIT_TLS":         &config.Mail.SMTP.ImplicitTLS,
		"SORO_SMTP_INSECURE_SKIP_VERIFY": &config.Mail.SMTP.InsecureSkipVerify,
	} {
		if value, ok := lookup(name); ok {
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("config: %s must be a boolean: %w", name, err)
			}
			*destination = parsed
		}
	}
	for name, destination := range map[string]*int{
		"SORO_JOBS_WORKERS": &config.Jobs.Workers,
		"SMTP_PORT":         &config.Mail.SMTP.Port,
	} {
		if value, ok := lookup(name); ok {
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("config: %s must be an integer: %w", name, err)
			}
			*destination = parsed
		}
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
		"SORO_HTTP_READ_HEADER_TIMEOUT":     &config.HTTP.ReadHeaderTimeout,
		"SORO_HTTP_READ_TIMEOUT":            &config.HTTP.ReadTimeout,
		"SORO_HTTP_WRITE_TIMEOUT":           &config.HTTP.WriteTimeout,
		"SORO_HTTP_IDLE_TIMEOUT":            &config.HTTP.IdleTimeout,
		"SORO_HTTP_SHUTDOWN_TIMEOUT":        &config.HTTP.ShutdownTimeout,
		"SORO_HTTP_READINESS_TIMEOUT":       &config.HTTP.ReadinessTimeout,
		"SORO_JOBS_SHUTDOWN_TIMEOUT":        &config.Jobs.ShutdownTimeout,
		"SORO_SMTP_TIMEOUT":                 &config.Mail.SMTP.Timeout,
		"SORO_OTEL_TIMEOUT":                 &config.Observability.OTLPTimeout,
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
