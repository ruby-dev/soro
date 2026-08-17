package repository

import (
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/ruby-dev/soro/model"
)

type modelMetadata struct {
	modelType      reflect.Type
	resourceName   string
	fieldsByName   map[string]fieldMetadata
	fieldsByColumn map[string]fieldMetadata
	updateColumns  []string
}

type fieldMetadata struct {
	name   string
	column string
}

func inspectModel[T any]() (*modelMetadata, error) {
	typeOf := reflect.TypeFor[T]()
	if typeOf.Kind() != reflect.Struct {
		return nil, fmt.Errorf("repository model must be a struct, got %s", typeOf)
	}
	value := reflect.New(typeOf).Interface()
	if _, ok := value.(model.Entity); !ok {
		return nil, fmt.Errorf("repository model %s must embed model.Base", typeOf)
	}
	metadata := &modelMetadata{
		modelType:      typeOf,
		resourceName:   typeOf.Name(),
		fieldsByName:   make(map[string]fieldMetadata),
		fieldsByColumn: make(map[string]fieldMetadata),
	}
	collectFields(typeOf, metadata)
	for _, field := range metadata.fieldsByName {
		switch field.column {
		case "id", "created_at", "created_by", "deleted_at", "deleted_by":
			continue
		}
		metadata.updateColumns = append(metadata.updateColumns, field.column)
	}
	return metadata, nil
}

func collectFields(typeOf reflect.Type, metadata *modelMetadata) {
	for index := range typeOf.NumField() {
		field := typeOf.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("bun")
		first := firstTagPart(tag)
		if first == "-" || strings.HasPrefix(first, "table:") {
			continue
		}
		if field.Anonymous && field.Type.Kind() == reflect.Struct && tag == "" {
			collectFields(field.Type, metadata)
			continue
		}
		if field.Anonymous && first == "" && field.Type.Kind() != reflect.Struct {
			continue
		}
		column := first
		if column == "" {
			column = snakeCase(field.Name)
		}
		entry := fieldMetadata{name: field.Name, column: column}
		metadata.fieldsByName[field.Name] = entry
		metadata.fieldsByColumn[column] = entry
	}
}

func (metadata *modelMetadata) column(field string) (string, bool) {
	if found, ok := metadata.fieldsByName[field]; ok {
		return found.column, true
	}
	if found, ok := metadata.fieldsByColumn[field]; ok {
		return found.column, true
	}
	return "", false
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
		if index > 0 && unicode.IsUpper(character) {
			result.WriteByte('_')
		}
		result.WriteRune(unicode.ToLower(character))
	}
	return result.String()
}
