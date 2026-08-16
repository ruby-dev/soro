package repository

import (
	"time"

	"github.com/datasoro/soro/lifecycle"
	"github.com/google/uuid"
)

type UUIDGenerator func() (uuid.UUID, error)
type Clock func() time.Time

type Option func(*settings)

type settings struct {
	clock         Clock
	uuidGenerator UUIDGenerator
}

func WithClock(clock Clock) Option {
	return func(settings *settings) { settings.clock = clock }
}

func WithUUIDGenerator(generator UUIDGenerator) Option {
	return func(settings *settings) { settings.uuidGenerator = generator }
}

type OperationOption func(*operationSettings)

type operationSettings struct {
	skipped  map[lifecycle.Stage]struct{}
	skipAll  bool
	metadata map[string]any
}

// SkipHooks deliberately bypasses only the named lifecycle stages.
func SkipHooks(stages ...lifecycle.Stage) OperationOption {
	return func(settings *operationSettings) {
		for _, stage := range stages {
			settings.skipped[stage] = struct{}{}
		}
	}
}

// SkipAllHooks deliberately bypasses every lifecycle hook. Validation still runs.
func SkipAllHooks() OperationOption {
	return func(settings *operationSettings) { settings.skipAll = true }
}

// WithLifecycleMetadata adds operation-local data visible to hooks.
func WithLifecycleMetadata(metadata map[string]any) OperationOption {
	return func(settings *operationSettings) {
		settings.metadata = make(map[string]any, len(metadata))
		for key, value := range metadata {
			settings.metadata[key] = value
		}
	}
}

func newOperationSettings(options []OperationOption) operationSettings {
	settings := operationSettings{skipped: make(map[lifecycle.Stage]struct{}), metadata: make(map[string]any)}
	for _, option := range options {
		option(&settings)
	}
	return settings
}

func (settings operationSettings) skips(stage lifecycle.Stage) bool {
	if settings.skipAll {
		return true
	}
	_, ok := settings.skipped[stage]
	return ok
}
