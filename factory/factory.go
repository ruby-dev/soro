// Package factory provides typed, persistence-agnostic test data factories.
package factory

import (
	"context"
	"fmt"
	"sync/atomic"
)

// Builder constructs an entity for a one-based sequence number.
type Builder[T any] func(sequence uint64) *T

// Trait applies an explicit variation to a built entity.
type Trait[T any] func(*T)

// Persister stores an entity. Applications normally adapt a repository Create
// method here instead of coupling factories to a persistence implementation.
type Persister[T any] func(context.Context, *T) error

type Factory[T any] struct {
	builder   Builder[T]
	persister Persister[T]
	sequence  atomic.Uint64
}

func New[T any](builder Builder[T], persister Persister[T]) (*Factory[T], error) {
	if builder == nil {
		return nil, fmt.Errorf("factory builder is required")
	}
	return &Factory[T]{builder: builder, persister: persister}, nil
}

// Build constructs an entity without persisting it. Traits run in call order.
func (factory *Factory[T]) Build(traits ...Trait[T]) (*T, error) {
	if factory == nil || factory.builder == nil {
		return nil, fmt.Errorf("factory is not initialized")
	}
	entity := factory.builder(factory.sequence.Add(1))
	if entity == nil {
		return nil, fmt.Errorf("factory builder returned nil")
	}
	for _, trait := range traits {
		if trait == nil {
			return nil, fmt.Errorf("factory trait cannot be nil")
		}
		trait(entity)
	}
	return entity, nil
}

// BuildList constructs count entities without persistence.
func (factory *Factory[T]) BuildList(count int, traits ...Trait[T]) ([]*T, error) {
	if count < 0 {
		return nil, fmt.Errorf("factory count cannot be negative")
	}
	entities := make([]*T, 0, count)
	for range count {
		entity, err := factory.Build(traits...)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, nil
}

// Create constructs and persists one entity.
func (factory *Factory[T]) Create(ctx context.Context, traits ...Trait[T]) (*T, error) {
	if factory == nil || factory.persister == nil {
		return nil, fmt.Errorf("factory persister is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entity, err := factory.Build(traits...)
	if err != nil {
		return nil, err
	}
	if err := factory.persister(ctx, entity); err != nil {
		return nil, fmt.Errorf("factory persist: %w", err)
	}
	return entity, nil
}

// CreateList constructs and persists count entities in order. Transactional
// behavior is deliberately owned by the supplied persister or calling context.
func (factory *Factory[T]) CreateList(ctx context.Context, count int, traits ...Trait[T]) ([]*T, error) {
	if count < 0 {
		return nil, fmt.Errorf("factory count cannot be negative")
	}
	entities := make([]*T, 0, count)
	for range count {
		entity, err := factory.Create(ctx, traits...)
		if err != nil {
			return nil, err
		}
		entities = append(entities, entity)
	}
	return entities, nil
}
