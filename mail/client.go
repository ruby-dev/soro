package mail

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/riverqueue/river"
	"github.com/ruby-dev/soro/jobs"
	"github.com/ruby-dev/soro/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const deliveryJobKind = "soro_mail_delivery"

type Config struct {
	DefaultFrom string
	Queue       string
}

type Client struct {
	transport     Transport
	transportName string
	jobs          *jobs.Client
	observer      *observability.Provider
	logger        *slog.Logger
	config        Config
}

func New(transport Transport, jobClient *jobs.Client, observer *observability.Provider, logger *slog.Logger, config Config) (*Client, error) {
	if transport == nil || observer == nil {
		return nil, fmt.Errorf("mail transport and observability provider are required")
	}
	if config.Queue == "" {
		config.Queue = "mailers"
	}
	if logger == nil {
		logger = slog.Default()
	}
	name := "custom"
	if named, ok := transport.(namedTransport); ok {
		name = named.Name()
	}
	client := &Client{transport: transport, transportName: name, jobs: jobClient, observer: observer, logger: logger, config: config}
	if jobClient != nil {
		if err := jobs.Register(jobClient, func(ctx context.Context, args deliveryJob) error { return client.Send(ctx, &args.Message) }); err != nil {
			return nil, fmt.Errorf("mail: register delivery worker: %w", err)
		}
	}
	return client, nil
}

func (client *Client) Delivery(message *Message) *Delivery {
	return &Delivery{client: client, message: message.Clone()}
}

func (client *Client) Send(ctx context.Context, message *Message) (sendErr error) {
	if message == nil {
		return fmt.Errorf("mail message is required")
	}
	prepared := message.Clone()
	if prepared.From == "" {
		prepared.From = client.config.DefaultFrom
	}
	if err := prepared.Validate(); err != nil {
		return err
	}
	ctx, span := client.observer.Tracer().Start(ctx, "mail send", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("mail.transport", client.transportName)))
	started := time.Now()
	defer func() {
		duration := time.Since(started)
		if sendErr != nil {
			span.RecordError(sendErr)
			span.SetStatus(codes.Error, "mail send failed")
		}
		span.End()
		client.observer.Metrics().RecordMail(ctx, client.transportName, duration, sendErr)
		if sendErr != nil {
			client.logger.ErrorContext(ctx, "mail send failed", "transport", client.transportName, "duration", duration, "error", sendErr)
			return
		}
		client.logger.InfoContext(ctx, "mail sent", "transport", client.transportName, "duration", duration)
	}()
	sendErr = client.transport.Send(ctx, prepared)
	return sendErr
}

type Delivery struct {
	client  *Client
	message *Message
}

func (delivery *Delivery) Send(ctx context.Context) error {
	if delivery == nil || delivery.client == nil {
		return fmt.Errorf("mail delivery client is required")
	}
	return delivery.client.Send(ctx, delivery.message)
}

func (delivery *Delivery) SendLater(ctx context.Context, options ...jobs.Option) (*jobs.Result, error) {
	if delivery == nil || delivery.client == nil || delivery.client.jobs == nil {
		return nil, fmt.Errorf("mail queued delivery requires jobs")
	}
	message := delivery.message.Clone()
	if message.From == "" {
		message.From = delivery.client.config.DefaultFrom
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}
	allOptions := append([]jobs.Option{jobs.Queue(delivery.client.config.Queue)}, options...)
	return delivery.client.jobs.Enqueue(ctx, deliveryJob{Message: *message}, allOptions...)
}

type deliveryJob struct {
	Message Message `json:"message"`
}

func (deliveryJob) Kind() string { return deliveryJobKind }

var _ river.JobArgs = deliveryJob{}
