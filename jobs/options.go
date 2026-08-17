package jobs

import (
	"fmt"
	"time"

	"github.com/riverqueue/river"
)

type Option func(*insertSettings) error

type insertSettings struct {
	queue       string
	delay       time.Duration
	priority    int
	maxAttempts int
	unique      river.UniqueOpts
}

func Queue(name string) Option {
	return func(settings *insertSettings) error {
		if name == "" {
			return fmt.Errorf("jobs queue cannot be empty")
		}
		settings.queue = name
		return nil
	}
}

func Delay(duration time.Duration) Option {
	return func(settings *insertSettings) error {
		if duration < 0 {
			return fmt.Errorf("jobs delay cannot be negative")
		}
		settings.delay = duration
		return nil
	}
}

func Priority(priority int) Option {
	return func(settings *insertSettings) error {
		if priority < 1 || priority > 4 {
			return fmt.Errorf("jobs priority must be between 1 and 4")
		}
		settings.priority = priority
		return nil
	}
}

func MaxAttempts(attempts int) Option {
	return func(settings *insertSettings) error {
		if attempts < 1 {
			return fmt.Errorf("jobs max attempts must be positive")
		}
		settings.maxAttempts = attempts
		return nil
	}
}

type UniqueConfig struct {
	ByArgs   bool
	ByQueue  bool
	ByPeriod time.Duration
}

func Unique(config UniqueConfig) Option {
	return func(settings *insertSettings) error {
		if config.ByPeriod < 0 {
			return fmt.Errorf("jobs unique period cannot be negative")
		}
		if !config.ByArgs && !config.ByQueue && config.ByPeriod == 0 {
			return fmt.Errorf("jobs unique requires at least one dimension")
		}
		settings.unique = river.UniqueOpts{ByArgs: config.ByArgs, ByQueue: config.ByQueue, ByPeriod: config.ByPeriod}
		return nil
	}
}

func UniqueByArgs() Option { return Unique(UniqueConfig{ByArgs: true}) }
