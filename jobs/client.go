// Package jobs wraps River with Soro transactions, options, and telemetry.
package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/ruby-dev/soro/database"
	"github.com/ruby-dev/soro/observability"
	"go.opentelemetry.io/otel/propagation"
)

type Config struct {
	WorkersEnabled  bool
	DefaultQueue    string
	Queues          map[string]int
	Schema          string
	ShutdownTimeout time.Duration
}

func (config Config) withDefaults() Config {
	if config.DefaultQueue == "" {
		config.DefaultQueue = river.QueueDefault
	}
	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = 30 * time.Second
	}
	if config.WorkersEnabled && len(config.Queues) == 0 {
		config.Queues = map[string]int{config.DefaultQueue: 10}
	}
	return config
}

func (config Config) Validate() error {
	config = config.withDefaults()
	if config.ShutdownTimeout <= 0 {
		return fmt.Errorf("jobs shutdown timeout must be positive")
	}
	for queue, workers := range config.Queues {
		if queue == "" || workers < 1 {
			return fmt.Errorf("jobs queues require a name and at least one worker")
		}
	}
	if config.WorkersEnabled {
		if _, exists := config.Queues[config.DefaultQueue]; !exists {
			return fmt.Errorf("jobs default queue must be configured for workers")
		}
	}
	return nil
}

type Client struct {
	db       *database.DB
	driver   *riverdatabasesql.Driver
	river    *river.Client[*sql.Tx]
	workers  *river.Workers
	observer *observability.Provider
	logger   *slog.Logger
	config   Config
	clock    func() time.Time
	startMu  sync.Mutex
	started  bool
}

type Result struct {
	ID        int64
	Duplicate bool
}

func New(db *database.DB, observer *observability.Provider, logger *slog.Logger, config Config) (*Client, error) {
	config = config.withDefaults()
	if db == nil || db.SQL() == nil {
		return nil, fmt.Errorf("jobs database is required")
	}
	if observer == nil {
		return nil, fmt.Errorf("jobs observability provider is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	workers := river.NewWorkers()
	var queues map[string]river.QueueConfig
	var configuredWorkers *river.Workers
	if config.WorkersEnabled {
		configuredWorkers = workers
		queues = make(map[string]river.QueueConfig, len(config.Queues))
		for name, count := range config.Queues {
			queues[name] = river.QueueConfig{MaxWorkers: count}
		}
	}
	driver := riverdatabasesql.New(db.SQL())
	riverClient, err := river.NewClient(driver, &river.Config{Logger: logger, Queues: queues, Workers: configuredWorkers, Schema: config.Schema})
	if err != nil {
		return nil, fmt.Errorf("jobs: create River client: %w", err)
	}
	return &Client{db: db, driver: driver, river: riverClient, workers: workers, observer: observer, logger: logger, config: config, clock: time.Now}, nil
}

func (client *Client) Migrate(ctx context.Context) error {
	migrator, err := rivermigrate.New(client.driver, &rivermigrate.Config{Logger: client.logger, Schema: client.config.Schema})
	if err != nil {
		return fmt.Errorf("jobs: create migrator: %w", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("jobs: migrate: %w", err)
	}
	return nil
}

func (client *Client) Enqueue(ctx context.Context, args river.JobArgs, options ...Option) (*Result, error) {
	if tx, ok := client.db.SQLTx(ctx); ok {
		return client.enqueue(ctx, tx, args, options...)
	}
	return client.enqueue(ctx, nil, args, options...)
}

func (client *Client) EnqueueTx(ctx context.Context, args river.JobArgs, options ...Option) (*Result, error) {
	tx, ok := client.db.SQLTx(ctx)
	if !ok {
		return nil, fmt.Errorf("jobs: transactional enqueue requires a Soro transaction context")
	}
	return client.enqueue(ctx, tx, args, options...)
}

func (client *Client) enqueue(ctx context.Context, tx *sql.Tx, args river.JobArgs, options ...Option) (*Result, error) {
	if args == nil || args.Kind() == "" {
		return nil, fmt.Errorf("jobs: arguments and kind are required")
	}
	settings := insertSettings{queue: client.config.DefaultQueue}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("jobs: nil enqueue option")
		}
		if err := option(&settings); err != nil {
			return nil, err
		}
	}
	metadata, err := client.traceMetadata(ctx)
	if err != nil {
		return nil, err
	}
	insertOptions := &river.InsertOpts{
		Queue: settings.queue, Priority: settings.priority, MaxAttempts: settings.maxAttempts,
		Metadata: metadata, UniqueOpts: settings.unique,
	}
	if settings.delay > 0 {
		insertOptions.ScheduledAt = client.clock().Add(settings.delay)
	}
	var inserted *rivertype.JobInsertResult
	if tx != nil {
		inserted, err = client.river.InsertTx(ctx, tx, args, insertOptions)
	} else {
		inserted, err = client.river.Insert(ctx, args, insertOptions)
	}
	if err != nil {
		return nil, fmt.Errorf("jobs: enqueue %s: %w", args.Kind(), err)
	}
	return &Result{ID: inserted.Job.ID, Duplicate: inserted.UniqueSkippedAsDuplicate}, nil
}

func (client *Client) traceMetadata(ctx context.Context) ([]byte, error) {
	carrier := propagation.MapCarrier{}
	client.observer.Propagator().Inject(ctx, carrier)
	if len(carrier) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(map[string]any{"soro_trace": map[string]string(carrier)})
	if err != nil {
		return nil, fmt.Errorf("jobs: encode trace metadata: %w", err)
	}
	return encoded, nil
}

func (client *Client) Start(ctx context.Context) error {
	if !client.config.WorkersEnabled {
		return nil
	}
	client.startMu.Lock()
	defer client.startMu.Unlock()
	if client.started {
		return nil
	}
	if err := client.river.Start(ctx); err != nil {
		return fmt.Errorf("jobs: start workers: %w", err)
	}
	client.started = true
	return nil
}

func (client *Client) Stop(ctx context.Context) error {
	client.startMu.Lock()
	started := client.started
	client.startMu.Unlock()
	if !started {
		return nil
	}
	if err := client.river.Stop(ctx); err != nil {
		return fmt.Errorf("jobs: stop workers: %w", err)
	}
	client.startMu.Lock()
	client.started = false
	client.startMu.Unlock()
	return nil
}

func (client *Client) Enabled() bool { return client.config.WorkersEnabled }
