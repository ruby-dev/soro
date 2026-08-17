package mail

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/datasoro/soro/observability"
)

func TestImmediateDeliveryAndCaptureIsolation(t *testing.T) {
	observer := mailTestObserver(t)
	capture := NewCaptureTransport()
	client, err := New(capture, nil, observer, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{DefaultFrom: "Soro <soro@example.com>"})
	if err != nil {
		t.Fatal(err)
	}
	message := &Message{To: []string{"user@example.com"}, Subject: "Welcome", Text: "Hello"}
	if err := client.Delivery(message).Send(t.Context()); err != nil {
		t.Fatal(err)
	}
	message.Subject = "mutated"
	messages := capture.Messages()
	if len(messages) != 1 || messages[0].From != "Soro <soro@example.com>" || messages[0].Subject != "Welcome" {
		t.Fatalf("captured messages = %+v", messages)
	}
	messages[0].Subject = "also mutated"
	if capture.Messages()[0].Subject != "Welcome" {
		t.Fatal("capture leaked mutable message")
	}
}

func TestTemplatesEscapeHTML(t *testing.T) {
	templates, err := ParseTemplates("welcome", "Welcome {{.Name}}", "Hello {{.Name}}", `<p>Hello {{.Name}}</p>`)
	if err != nil {
		t.Fatal(err)
	}
	subject, textBody, htmlBody, err := templates.Render(struct{ Name string }{Name: `<Dustin>`})
	if err != nil {
		t.Fatal(err)
	}
	if subject != "Welcome <Dustin>" || textBody != "Hello <Dustin>" || !strings.Contains(htmlBody, "&lt;Dustin&gt;") {
		t.Fatalf("rendered = %q %q %q", subject, textBody, htmlBody)
	}
}

func TestMessageValidationAndEncoding(t *testing.T) {
	invalid := &Message{From: "sender@example.com", To: []string{"bad\n@example.com"}, Subject: "subject", Text: "body"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected address validation error")
	}
	message := &Message{
		From: "sender@example.com", To: []string{"to@example.com"}, CC: []string{"cc@example.com"}, BCC: []string{"hidden@example.com"},
		Subject: "Welcome ✓", Text: "plain", HTML: "<strong>html</strong>",
	}
	encoded, err := encodeMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("hidden@example.com")) || !bytes.Contains(encoded, []byte("multipart/alternative")) {
		t.Fatalf("unexpected encoded message:\n%s", encoded)
	}
}

func TestSMTPConfigValidation(t *testing.T) {
	if _, err := NewSMTPTransport(SMTPConfig{}); err == nil {
		t.Fatal("expected host error")
	}
	if _, err := NewSMTPTransport(SMTPConfig{Host: "localhost", Port: 25, StartTLS: true, ImplicitTLS: true}); err == nil {
		t.Fatal("expected TLS mode error")
	}
	if _, err := NewSMTPTransport(SMTPConfig{Host: "localhost", Port: 25, Username: "user"}); err == nil {
		t.Fatal("expected credentials error")
	}
}

func TestConsoleAndCaptureReset(t *testing.T) {
	message := &Message{From: "sender@example.com", To: []string{"user@example.com"}, Subject: "Test", Text: "Body"}
	var output bytes.Buffer
	console := &ConsoleTransport{Writer: &output}
	if console.Name() != "console" {
		t.Fatalf("name = %q", console.Name())
	}
	if err := console.Send(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"subject":"Test"`) {
		t.Fatalf("console = %s", output.String())
	}
	capture := NewCaptureTransport()
	if err := capture.Send(t.Context(), message); err != nil {
		t.Fatal(err)
	}
	capture.Reset()
	if len(capture.Messages()) != 0 {
		t.Fatal("capture reset failed")
	}
	if got := message.recipients(); len(got) != 1 || got[0] != "user@example.com" {
		t.Fatalf("recipients = %v", got)
	}
}

func mailTestObserver(t *testing.T) *observability.Provider {
	t.Helper()
	provider, err := observability.New(t.Context(), observability.Config{ServiceName: "mail-test", Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return provider
}
