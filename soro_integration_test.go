package soro_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datasoro/soro"
	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/mail"
)

func TestApplicationServicesAndInfrastructureEndpoints(t *testing.T) {
	db := testdb.Open(t)
	settings := config.Defaults()
	settings.App.Environment = config.Test
	settings.Database.URL = "unused-with-injected-database"
	settings.Mail.Transport = "capture"
	capture := mail.NewCaptureTransport()
	app, err := soro.New(t.Context(), soro.WithConfig(&settings), soro.WithDatabase(db), soro.WithMailTransport(capture), soro.WithLogger(discardLogger()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})
	if app.Jobs == nil || app.Mailer == nil || app.Observability == nil || app.Health == nil {
		t.Fatal("Phase 3 services are missing")
	}
	if app.API.OpenAPI().Info.Version != settings.App.Version {
		t.Fatalf("OpenAPI version = %q, want %q", app.API.OpenAPI().Info.Version, settings.App.Version)
	}

	for path, status := range map[string]int{"/health": http.StatusOK, "/ready": http.StatusOK, "/metrics": http.StatusOK} {
		response := httptest.NewRecorder()
		app.API.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != status {
			t.Fatalf("%s status = %d body=%s", path, response.Code, response.Body.String())
		}
	}
	if err := app.Mailer.Delivery(&mail.Message{To: []string{"user@example.com"}, Subject: "Test", Text: "Body"}).Send(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(capture.Messages()) != 1 {
		t.Fatalf("captured messages = %d", len(capture.Messages()))
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServeShutsDownWhenContextIsCancelled(t *testing.T) {
	db := testdb.Open(t)
	settings := config.Defaults()
	settings.App.Environment = config.Test
	settings.Database.URL = "unused-with-injected-database"
	settings.HTTP.Port = availablePort(t)
	settings.HTTP.ShutdownTimeout = 5 * time.Second
	app, err := soro.New(t.Context(), soro.WithConfig(&settings), soro.WithDatabase(db), soro.WithMailTransport(mail.NewCaptureTransport()), soro.WithLogger(discardLogger()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- app.Serve(ctx) }()
	address := "http://127.0.0.1:" + fmt.Sprint(settings.HTTP.Port) + "/health"
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, requestErr := http.Get(address) //nolint:gosec
		if requestErr == nil {
			response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", requestErr)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
	if err := app.Close(); err != nil {
		t.Fatal(err)
	}
}

func availablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
