package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/datasoro/soro/health"
)

func TestHealthAndReadiness(t *testing.T) {
	registry, err := health.New(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("database", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("jobs", func(context.Context) error { return errors.New("down") }); err != nil {
		t.Fatal(err)
	}

	healthResponse := httptest.NewRecorder()
	registry.HealthHandler().ServeHTTP(healthResponse, httptest.NewRequest(http.MethodGet, "/health", nil))
	if healthResponse.Code != http.StatusOK || !strings.Contains(healthResponse.Body.String(), `"status":"ok"`) {
		t.Fatalf("health response = %d %s", healthResponse.Code, healthResponse.Body.String())
	}
	readyResponse := httptest.NewRecorder()
	registry.ReadyHandler().ServeHTTP(readyResponse, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if readyResponse.Code != http.StatusServiceUnavailable || !strings.Contains(readyResponse.Body.String(), `"jobs":{"status":"failed"}`) {
		t.Fatalf("ready response = %d %s", readyResponse.Code, readyResponse.Body.String())
	}
}

func TestRegistrationValidation(t *testing.T) {
	if _, err := health.New(0); err == nil {
		t.Fatal("expected timeout validation")
	}
	registry, _ := health.New(time.Second)
	check := func(context.Context) error { return nil }
	if err := registry.Register("database", check); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("database", check); err == nil {
		t.Fatal("expected duplicate check error")
	}
}
