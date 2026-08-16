package lifecycle

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

type Handler func(context.Context, any, *Context) error

type registration struct {
	priority int
	sequence uint64
	handler  Handler
}

// Registry stores deterministic global and model-specific hooks.
type Registry struct {
	mu       sync.RWMutex
	sequence uint64
	global   map[Stage][]registration
	models   map[reflect.Type]map[Stage][]registration
}

func NewRegistry() *Registry {
	return &Registry{
		global: make(map[Stage][]registration),
		models: make(map[reflect.Type]map[Stage][]registration),
	}
}

func (r *Registry) RegisterGlobal(stage Stage, priority int, handler Handler) error {
	if err := validateRegistration(stage, handler); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	r.global[stage] = append(r.global[stage], registration{priority: priority, sequence: r.sequence, handler: handler})
	sortRegistrations(r.global[stage])
	return nil
}

func (r *Registry) Register(model any, stage Stage, priority int, handler Handler) error {
	if err := validateRegistration(stage, handler); err != nil {
		return err
	}
	modelType, err := concreteType(model)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	if r.models[modelType] == nil {
		r.models[modelType] = make(map[Stage][]registration)
	}
	r.models[modelType][stage] = append(r.models[modelType][stage], registration{priority: priority, sequence: r.sequence, handler: handler})
	sortRegistrations(r.models[modelType][stage])
	return nil
}

// Run invokes one stage. Before stages run global, registered, model hooks;
// after stages run model, registered, global hooks.
func (r *Registry) Run(ctx context.Context, stage Stage, entity any, lifecycle *Context) error {
	if lifecycle == nil {
		return fmt.Errorf("lifecycle context is required")
	}
	global, registered := r.handlers(entity, stage)
	if isBefore(stage) {
		if err := runHandlers(ctx, global, entity, lifecycle); err != nil {
			return err
		}
		if err := runHandlers(ctx, registered, entity, lifecycle); err != nil {
			return err
		}
		return runModelHook(ctx, stage, entity, lifecycle)
	}
	if err := runModelHook(ctx, stage, entity, lifecycle); err != nil {
		return err
	}
	if err := runHandlers(ctx, registered, entity, lifecycle); err != nil {
		return err
	}
	return runHandlers(ctx, global, entity, lifecycle)
}

func (r *Registry) handlers(entity any, stage Stage) ([]registration, []registration) {
	typeOf, _ := concreteType(entity)
	r.mu.RLock()
	defer r.mu.RUnlock()
	global := append([]registration(nil), r.global[stage]...)
	var registered []registration
	if byStage := r.models[typeOf]; byStage != nil {
		registered = append([]registration(nil), byStage[stage]...)
	}
	return global, registered
}

func runHandlers(ctx context.Context, handlers []registration, entity any, lifecycle *Context) error {
	for _, registered := range handlers {
		if err := registered.handler(ctx, entity, lifecycle); err != nil {
			return err
		}
	}
	return nil
}

func runModelHook(ctx context.Context, stage Stage, entity any, lifecycle *Context) error {
	switch stage {
	case BeforeValidate:
		if hook, ok := entity.(BeforeValidateHook); ok {
			return hook.BeforeValidate(ctx, lifecycle)
		}
	case AfterValidate:
		if hook, ok := entity.(AfterValidateHook); ok {
			return hook.AfterValidate(ctx, lifecycle)
		}
	case BeforeSave:
		if hook, ok := entity.(BeforeSaveHook); ok {
			return hook.BeforeSave(ctx, lifecycle)
		}
	case AfterSave:
		if hook, ok := entity.(AfterSaveHook); ok {
			return hook.AfterSave(ctx, lifecycle)
		}
	case BeforeCreate:
		if hook, ok := entity.(BeforeCreateHook); ok {
			return hook.BeforeCreate(ctx, lifecycle)
		}
	case AfterCreate:
		if hook, ok := entity.(AfterCreateHook); ok {
			return hook.AfterCreate(ctx, lifecycle)
		}
	case BeforeUpdate:
		if hook, ok := entity.(BeforeUpdateHook); ok {
			return hook.BeforeUpdate(ctx, lifecycle)
		}
	case AfterUpdate:
		if hook, ok := entity.(AfterUpdateHook); ok {
			return hook.AfterUpdate(ctx, lifecycle)
		}
	case BeforeDelete:
		if hook, ok := entity.(BeforeDeleteHook); ok {
			return hook.BeforeDelete(ctx, lifecycle)
		}
	case AfterDelete:
		if hook, ok := entity.(AfterDeleteHook); ok {
			return hook.AfterDelete(ctx, lifecycle)
		}
	case BeforeRestore:
		if hook, ok := entity.(BeforeRestoreHook); ok {
			return hook.BeforeRestore(ctx, lifecycle)
		}
	case AfterRestore:
		if hook, ok := entity.(AfterRestoreHook); ok {
			return hook.AfterRestore(ctx, lifecycle)
		}
	case BeforeForceDelete:
		if hook, ok := entity.(BeforeForceDeleteHook); ok {
			return hook.BeforeForceDelete(ctx, lifecycle)
		}
	case AfterForceDelete:
		if hook, ok := entity.(AfterForceDeleteHook); ok {
			return hook.AfterForceDelete(ctx, lifecycle)
		}
	case AfterCommit:
		if hook, ok := entity.(AfterCommitHook); ok {
			return hook.AfterCommit(ctx, lifecycle)
		}
	case AfterRollback:
		if hook, ok := entity.(AfterRollbackHook); ok {
			return hook.AfterRollback(ctx, lifecycle)
		}
	default:
		return fmt.Errorf("unknown lifecycle stage %q", stage)
	}
	return nil
}

func validateRegistration(stage Stage, handler Handler) error {
	if !knownStage(stage) {
		return fmt.Errorf("unknown lifecycle stage %q", stage)
	}
	if handler == nil {
		return fmt.Errorf("lifecycle handler is required")
	}
	return nil
}

func concreteType(value any) (reflect.Type, error) {
	if value == nil {
		return nil, fmt.Errorf("model type is required")
	}
	typeOf := reflect.TypeOf(value)
	for typeOf.Kind() == reflect.Pointer {
		typeOf = typeOf.Elem()
	}
	if typeOf.Kind() != reflect.Struct {
		return nil, fmt.Errorf("model must be a struct or pointer to struct, got %s", typeOf)
	}
	return typeOf, nil
}

func sortRegistrations(registrations []registration) {
	sort.SliceStable(registrations, func(i, j int) bool {
		if registrations[i].priority != registrations[j].priority {
			return registrations[i].priority < registrations[j].priority
		}
		return registrations[i].sequence < registrations[j].sequence
	})
}

func isBefore(stage Stage) bool {
	switch stage {
	case BeforeValidate, BeforeSave, BeforeCreate, BeforeUpdate, BeforeDelete, BeforeRestore, BeforeForceDelete:
		return true
	default:
		return false
	}
}

func knownStage(stage Stage) bool {
	switch stage {
	case BeforeValidate, AfterValidate, BeforeSave, AfterSave, BeforeCreate, AfterCreate,
		BeforeUpdate, AfterUpdate, BeforeDelete, AfterDelete, BeforeRestore, AfterRestore,
		BeforeForceDelete, AfterForceDelete, AfterCommit, AfterRollback:
		return true
	default:
		return false
	}
}
