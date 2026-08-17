// Package serializer keeps persistence models separate from public responses.
package serializer

import "context"

type Serializer[T, R any] interface {
	Serialize(context.Context, *T) (R, error)
}

type CollectionSerializer[T, R any] interface {
	SerializeCollection(context.Context, []*T) ([]R, error)
}

type Func[T, R any] func(context.Context, *T) (R, error)

func (function Func[T, R]) Serialize(ctx context.Context, entity *T) (R, error) {
	return function(ctx, entity)
}

// Collection uses a serializer's optimized collection implementation when
// available and otherwise preserves input order while serializing each item.
func Collection[T, R any](ctx context.Context, configured Serializer[T, R], entities []*T) ([]R, error) {
	if batch, ok := configured.(CollectionSerializer[T, R]); ok {
		return batch.SerializeCollection(ctx, entities)
	}
	responses := make([]R, 0, len(entities))
	for _, entity := range entities {
		response, err := configured.Serialize(ctx, entity)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}
