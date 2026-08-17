package api

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/ruby-dev/soro/query"
	"github.com/ruby-dev/soro/repository"
	"github.com/ruby-dev/soro/serializer"
	"github.com/uptrace/bun"
)

// Action identifies one of the standard resource operations.
type Action string

const (
	Index   Action = "index"
	Show    Action = "show"
	Create  Action = "create"
	Update  Action = "update"
	Destroy Action = "destroy"
)

type ResourceHook[T any] func(context.Context, Action, *T) error

// ResourceConfig describes a typed REST resource. CreateEntity and UpdateEntity
// are deliberately explicit so API input can never be reflectively assigned to
// a persistence model.
type ResourceConfig[T, C, U, R any] struct {
	Name            string
	Repository      *repository.Repository[T]
	Serializer      serializer.Serializer[T, R]
	CreateEntity    func(context.Context, C) (*T, error)
	UpdateEntity    func(context.Context, *T, U) error
	Query           query.Definition
	Disabled        []Action
	Authorize       ResourceHook[T]
	Before          ResourceHook[T]
	After           ResourceHook[T]
	Scope           func(context.Context, *bun.SelectQuery) *bun.SelectQuery
	ModifyOperation map[Action]func(*huma.Operation)
}

// Resource implements the five conventional REST operations for one model.
type Resource[T, C, U, R any] struct {
	config ResourceConfig[T, C, U, R]
}

func NewResource[T, C, U, R any](config ResourceConfig[T, C, U, R]) (*Resource[T, C, U, R], error) {
	if strings.TrimSpace(config.Name) == "" {
		return nil, fmt.Errorf("resource name is required")
	}
	if config.Repository == nil || config.Serializer == nil {
		return nil, fmt.Errorf("resource repository and serializer are required")
	}
	if !slices.Contains(config.Disabled, Create) && config.CreateEntity == nil {
		return nil, fmt.Errorf("resource create entity function is required")
	}
	if !slices.Contains(config.Disabled, Update) && config.UpdateEntity == nil {
		return nil, fmt.Errorf("resource update entity function is required")
	}
	if err := config.Query.Validate(); err != nil {
		return nil, fmt.Errorf("resource query: %w", err)
	}
	validActions := map[Action]bool{Index: true, Show: true, Create: true, Update: true, Destroy: true}
	seen := make(map[Action]bool, len(config.Disabled))
	for _, action := range config.Disabled {
		if !validActions[action] {
			return nil, fmt.Errorf("resource has unknown disabled action %q", action)
		}
		if seen[action] {
			return nil, fmt.Errorf("resource disabled action %q is duplicated", action)
		}
		seen[action] = true
	}
	return &Resource[T, C, U, R]{config: config}, nil
}

type listInput struct {
	values url.Values
}

func (input *listInput) Resolve(ctx huma.Context) []error {
	requestURL := ctx.URL()
	input.values = requestURL.Query()
	return nil
}

type idInput struct {
	ID uuid.UUID `path:"id" doc:"Resource UUID"`
}

type createInput[C any] struct {
	Body C
}

type updateInput[U any] struct {
	ID   uuid.UUID `path:"id" doc:"Resource UUID"`
	Body U
}

type dataOutput[R any] struct {
	Body struct {
		Data R `json:"data"`
	}
}

type listOutput[R any] struct {
	Body struct {
		Data []R            `json:"data"`
		Meta PaginationMeta `json:"meta"`
	}
}

type emptyOutput struct{}

type PaginationMeta struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	Total   int `json:"total"`
	Pages   int `json:"pages"`
}

func (resource *Resource[T, C, U, R]) Register(router *Router, path string) {
	tag := resource.config.Name
	slug := strings.ReplaceAll(strings.Trim(path, "/"), "/", "-")
	if !resource.disabled(Index) {
		operation := resource.operation(Index, http.MethodGet, path, "List "+tag, "list-"+slug)
		operation.Parameters = append(operation.Parameters, queryParameters(resource.config.Query)...)
		Register(router, operation, resource.index)
	}
	if !resource.disabled(Show) {
		Register(router, resource.operation(Show, http.MethodGet, path+"/{id}", "Get "+tag, "get-"+singular(slug)), resource.show)
	}
	if !resource.disabled(Create) {
		op := resource.operation(Create, http.MethodPost, path, "Create "+tag, "create-"+singular(slug))
		op.DefaultStatus = http.StatusCreated
		Register(router, op, resource.create)
	}
	if !resource.disabled(Update) {
		Register(router, resource.operation(Update, http.MethodPatch, path+"/{id}", "Update "+tag, "update-"+singular(slug)), resource.update)
	}
	if !resource.disabled(Destroy) {
		op := resource.operation(Destroy, http.MethodDelete, path+"/{id}", "Delete "+tag, "delete-"+singular(slug))
		op.DefaultStatus = http.StatusNoContent
		Register(router, op, resource.destroy)
	}
}

func (resource *Resource[T, C, U, R]) operation(action Action, method, path, summary, operationID string) huma.Operation {
	operation := huma.Operation{Method: method, Path: path, Summary: summary, OperationID: operationID, Tags: []string{resource.config.Name}}
	if modify := resource.config.ModifyOperation[action]; modify != nil {
		modify(&operation)
	}
	return operation
}

func (resource *Resource[T, C, U, R]) index(ctx context.Context, input *listInput) (*listOutput[R], error) {
	if err := resource.run(resource.config.Authorize, ctx, Index, nil); err != nil {
		return nil, err
	}
	params, err := query.Parse(input.values, resource.config.Query)
	if err != nil {
		return nil, err
	}
	selectQuery := resource.config.Repository.Query(ctx)
	if resource.config.Scope != nil {
		selectQuery = resource.config.Scope(ctx, selectQuery)
		if selectQuery == nil {
			return nil, fmt.Errorf("resource scope returned a nil query")
		}
	}
	selectQuery = query.Apply(selectQuery, params, resource.config.Query)
	total, err := selectQuery.Clone().Count(ctx)
	if err != nil {
		return nil, err
	}
	entities := make([]*T, 0)
	if err := query.ApplyPagination(selectQuery, params).Model(&entities).Scan(ctx); err != nil {
		return nil, err
	}
	data, err := serializer.Collection(ctx, resource.config.Serializer, entities)
	if err != nil {
		return nil, err
	}
	output := &listOutput[R]{}
	output.Body.Data = data
	output.Body.Meta = PaginationMeta{Page: params.Page, PerPage: params.PerPage, Total: total}
	if total > 0 {
		output.Body.Meta.Pages = int(math.Ceil(float64(total) / float64(params.PerPage)))
	}
	return output, nil
}

func (resource *Resource[T, C, U, R]) show(ctx context.Context, input *idInput) (*dataOutput[R], error) {
	entity, err := resource.config.Repository.Find(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if err := resource.run(resource.config.Authorize, ctx, Show, entity); err != nil {
		return nil, err
	}
	return resource.serialize(ctx, entity)
}

func (resource *Resource[T, C, U, R]) create(ctx context.Context, input *createInput[C]) (*dataOutput[R], error) {
	var entity *T
	err := resource.config.Repository.Transaction(ctx, func(txContext context.Context, _ *repository.Repository[T]) error {
		var err error
		entity, err = resource.config.CreateEntity(txContext, input.Body)
		if err != nil {
			return err
		}
		if entity == nil {
			return fmt.Errorf("resource create entity function returned nil")
		}
		for _, hook := range []ResourceHook[T]{resource.config.Authorize, resource.config.Before} {
			if err := resource.run(hook, txContext, Create, entity); err != nil {
				return err
			}
		}
		if err := resource.config.Repository.Create(txContext, entity); err != nil {
			return err
		}
		return resource.run(resource.config.After, txContext, Create, entity)
	})
	if err != nil {
		return nil, err
	}
	return resource.serialize(ctx, entity)
}

func (resource *Resource[T, C, U, R]) update(ctx context.Context, input *updateInput[U]) (*dataOutput[R], error) {
	var entity *T
	err := resource.config.Repository.Transaction(ctx, func(txContext context.Context, _ *repository.Repository[T]) error {
		var err error
		entity, err = resource.config.Repository.Find(txContext, input.ID)
		if err != nil {
			return err
		}
		for _, hook := range []ResourceHook[T]{resource.config.Authorize, resource.config.Before} {
			if err := resource.run(hook, txContext, Update, entity); err != nil {
				return err
			}
		}
		if err := resource.config.UpdateEntity(txContext, entity, input.Body); err != nil {
			return err
		}
		if err := resource.config.Repository.Update(txContext, entity); err != nil {
			return err
		}
		return resource.run(resource.config.After, txContext, Update, entity)
	})
	if err != nil {
		return nil, err
	}
	return resource.serialize(ctx, entity)
}

func (resource *Resource[T, C, U, R]) destroy(ctx context.Context, input *idInput) (*emptyOutput, error) {
	err := resource.config.Repository.Transaction(ctx, func(txContext context.Context, _ *repository.Repository[T]) error {
		entity, err := resource.config.Repository.Find(txContext, input.ID)
		if err != nil {
			return err
		}
		for _, hook := range []ResourceHook[T]{resource.config.Authorize, resource.config.Before} {
			if err := resource.run(hook, txContext, Destroy, entity); err != nil {
				return err
			}
		}
		if err := resource.config.Repository.Delete(txContext, input.ID); err != nil {
			return err
		}
		return resource.run(resource.config.After, txContext, Destroy, entity)
	})
	if err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (resource *Resource[T, C, U, R]) serialize(ctx context.Context, entity *T) (*dataOutput[R], error) {
	data, err := resource.config.Serializer.Serialize(ctx, entity)
	if err != nil {
		return nil, err
	}
	output := &dataOutput[R]{}
	output.Body.Data = data
	return output, nil
}

func (resource *Resource[T, C, U, R]) run(hook ResourceHook[T], ctx context.Context, action Action, entity *T) error {
	if hook == nil {
		return nil
	}
	return hook(ctx, action, entity)
}

func (resource *Resource[T, C, U, R]) disabled(action Action) bool {
	return slices.Contains(resource.config.Disabled, action)
}

func singular(value string) string {
	return strings.TrimSuffix(value, "s")
}
