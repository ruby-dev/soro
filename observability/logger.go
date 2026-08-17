package observability

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/ruby-dev/soro/config"
)

const RedactedValue = "[REDACTED]"

type LoggerConfig struct {
	Writer        io.Writer
	Environment   string
	Level         string
	Format        string
	SensitiveKeys []string
	ReplaceAttr   func(groups []string, attr slog.Attr) slog.Attr
}

// NewLogger creates the framework's default structured logger. Auto format is
// readable text outside production and JSON in production.
func NewLogger(settings LoggerConfig) (*slog.Logger, error) {
	if settings.Writer == nil {
		return nil, fmt.Errorf("logger writer is required")
	}
	level, err := logLevel(settings.Level)
	if err != nil {
		return nil, err
	}
	format := settings.Format
	if format == "" || format == "auto" {
		format = "text"
		if settings.Environment == config.Production {
			format = "json"
		}
	}
	if format != "text" && format != "json" {
		return nil, fmt.Errorf("unsupported log format %q", settings.Format)
	}
	sensitive := defaultSensitiveKeys()
	for _, key := range settings.SensitiveKeys {
		normalized := normalizeLogKey(key)
		if normalized == "" {
			return nil, fmt.Errorf("sensitive log key cannot be empty")
		}
		sensitive[normalized] = struct{}{}
	}
	options := &slog.HandlerOptions{Level: level, ReplaceAttr: func(groups []string, attr slog.Attr) slog.Attr {
		if settings.ReplaceAttr != nil {
			attr = settings.ReplaceAttr(groups, attr)
			if attr.Equal(slog.Attr{}) {
				return attr
			}
		}
		if _, exists := sensitive[normalizeLogKey(attr.Key)]; exists {
			attr.Value = slog.StringValue(RedactedValue)
		}
		return attr
	}}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(settings.Writer, options)), nil
	}
	return slog.New(slog.NewTextHandler(settings.Writer, options)), nil
}

func logLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}

func defaultSensitiveKeys() map[string]struct{} {
	result := make(map[string]struct{})
	for _, key := range []string{
		"password", "passphrase", "secret", "authorization", "cookie", "set-cookie",
		"api-key", "access-token", "refresh-token", "client-secret",
	} {
		result[normalizeLogKey(key)] = struct{}{}
	}
	return result
}

func normalizeLogKey(value string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'A' && character <= 'Z' {
			return character + ('a' - 'A')
		}
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			return character
		}
		return -1
	}, value)
}
