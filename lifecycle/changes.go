package lifecycle

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

type Change struct {
	Field  string
	Column string
	Old    any
	New    any
}

type Changes struct {
	values map[string]Change
}

func Compare(oldValue, newValue any) (Changes, error) {
	oldStruct, err := indirectStruct(oldValue)
	if err != nil {
		return Changes{}, fmt.Errorf("old value: %w", err)
	}
	newStruct, err := indirectStruct(newValue)
	if err != nil {
		return Changes{}, fmt.Errorf("new value: %w", err)
	}
	if oldStruct.Type() != newStruct.Type() {
		return Changes{}, fmt.Errorf("cannot compare %s with %s", oldStruct.Type(), newStruct.Type())
	}
	changes := Changes{values: make(map[string]Change)}
	compareStruct(oldStruct, newStruct, &changes)
	return changes, nil
}

func (c Changes) Changed(field string) bool {
	_, ok := c.values[field]
	return ok
}

func (c Changes) HasChanges() bool { return len(c.values) > 0 }

func (c Changes) Fields() []string {
	fields := make([]string, 0, len(c.values))
	for field := range c.values {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func (c Changes) Values(field string) (oldValue, newValue any, ok bool) {
	change, ok := c.values[field]
	if !ok {
		return nil, nil, false
	}
	return cloneValue(change.Old), cloneValue(change.New), true
}

func (c Changes) Change(field string) (Change, bool) {
	change, ok := c.values[field]
	if !ok {
		return Change{}, false
	}
	change.Old = cloneValue(change.Old)
	change.New = cloneValue(change.New)
	return change, true
}

func (c Changes) Was(field string, expected any) bool {
	change, ok := c.values[field]
	return ok && reflect.DeepEqual(change.Old, expected)
}

func (c Changes) Is(field string, expected any) bool {
	change, ok := c.values[field]
	return ok && reflect.DeepEqual(change.New, expected)
}

func compareStruct(oldStruct, newStruct reflect.Value, changes *Changes) {
	typeOf := oldStruct.Type()
	for index := range oldStruct.NumField() {
		field := typeOf.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("bun")
		if firstTagPart(tag) == "-" {
			continue
		}
		oldField := oldStruct.Field(index)
		newField := newStruct.Field(index)
		if field.Anonymous && field.Type.Kind() == reflect.Struct && tag == "" {
			compareStruct(oldField, newField, changes)
			continue
		}
		oldInterface := normalizedValue(oldField)
		newInterface := normalizedValue(newField)
		if reflect.DeepEqual(oldInterface, newInterface) {
			continue
		}
		column := firstTagPart(tag)
		if column == "" {
			column = snakeCase(field.Name)
		}
		changes.values[field.Name] = Change{
			Field: field.Name, Column: column,
			Old: cloneValue(oldInterface), New: cloneValue(newInterface),
		}
	}
}

func indirectStruct(value any) (reflect.Value, error) {
	if value == nil {
		return reflect.Value{}, fmt.Errorf("value is nil")
	}
	result := reflect.ValueOf(value)
	for result.Kind() == reflect.Pointer {
		if result.IsNil() {
			return reflect.Value{}, fmt.Errorf("value is a nil pointer")
		}
		result = result.Elem()
	}
	if result.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("value must be a struct, got %s", result.Kind())
	}
	return result, nil
}

func normalizedValue(value reflect.Value) any {
	result := value.Interface()
	switch typed := result.(type) {
	case time.Time:
		return typed.UTC()
	case *time.Time:
		if typed == nil {
			return (*time.Time)(nil)
		}
		normalized := typed.UTC()
		return &normalized
	default:
		return result
	}
}

func cloneValue(value any) any {
	return cloneReflect(reflect.ValueOf(value)).Interface()
}

func cloneReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Zero(reflect.TypeFor[any]())
	}
	switch value.Kind() {
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			cloned.SetMapIndex(iterator.Key(), cloneReflect(iterator.Value()))
		}
		return cloned
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := range value.Len() {
			cloned.Index(index).Set(cloneReflect(value.Index(index)))
		}
		return cloned
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := reflect.New(value.Type().Elem())
		cloned.Elem().Set(cloneReflect(value.Elem()))
		return cloned
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneReflect(value.Elem())
		wrapped := reflect.New(value.Type()).Elem()
		wrapped.Set(cloned)
		return wrapped
	default:
		return value
	}
}

func firstTagPart(tag string) string {
	if before, _, ok := strings.Cut(tag, ","); ok {
		return before
	}
	return tag
}

func snakeCase(value string) string {
	var result strings.Builder
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}
