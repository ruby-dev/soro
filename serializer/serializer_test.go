package serializer_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/ruby-dev/soro/serializer"
)

func TestFuncAndCollectionPreserveOrder(t *testing.T) {
	type entity struct{ Value int }
	configured := serializer.Func[entity, int](func(_ context.Context, entity *entity) (int, error) {
		return entity.Value, nil
	})
	responses, err := serializer.Collection(t.Context(), configured, []*entity{{Value: 2}, {Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(responses, []int{2, 1}) {
		t.Fatalf("responses = %v", responses)
	}
}

func TestCollectionStopsAtFirstError(t *testing.T) {
	sentinel := errors.New("serialize")
	type entity struct{ Value int }
	configured := serializer.Func[entity, int](func(_ context.Context, entity *entity) (int, error) {
		if entity.Value == 2 {
			return 0, sentinel
		}
		return entity.Value, nil
	})
	_, err := serializer.Collection(t.Context(), configured, []*entity{{Value: 1}, {Value: 2}, {Value: 3}})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected serializer error, got %v", err)
	}
}
