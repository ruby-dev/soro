package mail_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/ruby-dev/soro/internal/testdb"
	"github.com/ruby-dev/soro/jobs"
	"github.com/ruby-dev/soro/mail"
	"github.com/ruby-dev/soro/observability"
)

func TestSendLaterWaitsForTransactionCommit(t *testing.T) {
	db := testdb.Open(t)
	observer, err := observability.New(t.Context(), observability.Config{ServiceName: "mail-integration", Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := observer.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jobClient, err := jobs.New(db, observer, logger, jobs.Config{
		WorkersEnabled: true, Queues: map[string]int{"default": 1, "mailers": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := jobClient.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	capture := mail.NewCaptureTransport()
	mailer, err := mail.New(capture, jobClient, observer, logger, mail.Config{DefaultFrom: "soro@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	workerContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := jobClient.Start(workerContext); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := jobClient.Stop(ctx); err != nil {
			t.Error(err)
		}
	})

	err = db.Transaction(t.Context(), func(ctx context.Context) error {
		_, err := mailer.Delivery(&mail.Message{To: []string{"user@example.com"}, Subject: "Queued", Text: "Hello"}).SendLater(ctx)
		if err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond)
		if len(capture.Messages()) != 0 {
			t.Fatal("mail delivered before transaction commit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	for len(capture.Messages()) == 0 {
		select {
		case <-deadline:
			t.Fatal("queued mail was not delivered")
		case <-time.After(25 * time.Millisecond):
		}
	}
	if capture.Messages()[0].Subject != "Queued" {
		t.Fatalf("messages = %+v", capture.Messages())
	}
}
