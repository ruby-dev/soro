package observability_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/ruby-dev/soro/config"
	"github.com/ruby-dev/soro/observability"
)

func TestNewLoggerFormatsByEnvironmentAndRedacts(t *testing.T) {
	var production bytes.Buffer
	logger, err := observability.NewLogger(observability.LoggerConfig{
		Writer: &production, Environment: config.Production, Level: "debug", Format: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("configured", "password", "danger", "token_count", 12)
	var record map[string]any
	if err := json.Unmarshal(production.Bytes(), &record); err != nil {
		t.Fatalf("production log is not JSON: %v: %s", err, production.String())
	}
	if record["password"] != observability.RedactedValue || record["token_count"] != float64(12) {
		t.Fatalf("unexpected production record: %#v", record)
	}

	var development bytes.Buffer
	logger, err = observability.NewLogger(observability.LoggerConfig{
		Writer: &development, Environment: config.Development, Level: "info", Format: "auto",
		SensitiveKeys: []string{"private_note"},
	})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("readable", "private_note", "hidden")
	if output := development.String(); !strings.Contains(output, "msg=readable") || !strings.Contains(output, `private_note=[REDACTED]`) || strings.Contains(output, "hidden") {
		t.Fatalf("unexpected development log: %s", output)
	}
}

func TestNewLoggerValidationAndReplacement(t *testing.T) {
	for _, settings := range []observability.LoggerConfig{
		{},
		{Writer: &bytes.Buffer{}, Level: "trace"},
		{Writer: &bytes.Buffer{}, Format: "xml"},
		{Writer: &bytes.Buffer{}, SensitiveKeys: []string{"---"}},
	} {
		if _, err := observability.NewLogger(settings); err == nil {
			t.Fatalf("expected validation error for %#v", settings)
		}
	}
	var output bytes.Buffer
	logger, err := observability.NewLogger(observability.LoggerConfig{
		Writer: &output, Format: "json",
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == "remove" {
				return slog.Attr{}
			}
			return attr
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("test", "remove", "value", "api_key", "secret")
	if strings.Contains(output.String(), "remove") || strings.Contains(output.String(), "secret") {
		t.Fatalf("replacement/redaction failed: %s", output.String())
	}
}
