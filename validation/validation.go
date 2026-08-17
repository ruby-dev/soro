// Package validation provides Soro's HTTP-independent validation engine.
package validation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	playground "github.com/go-playground/validator/v10"
	soroerrors "github.com/ruby-dev/soro/errors"
)

type Validator interface {
	Validate(context.Context) error
}

type Engine struct {
	validator *playground.Validate
}

func New() *Engine {
	engine := playground.New(playground.WithRequiredStructEnabled())
	engine.RegisterTagNameFunc(func(field reflect.StructField) string {
		if name := strings.SplitN(field.Tag.Get("json"), ",", 2)[0]; name != "" && name != "-" {
			return name
		}
		return snakeCase(field.Name)
	})
	return &Engine{validator: engine}
}

func (e *Engine) Validate(ctx context.Context, value any) error {
	if value == nil {
		return soroerrors.Validation(map[string][]string{"_base": {"must be present"}})
	}
	if contextual, ok := value.(Validator); ok {
		if err := contextual.Validate(ctx); err != nil {
			var frameworkError *soroerrors.Error
			if errors.As(err, &frameworkError) {
				return err
			}
			return soroerrors.Validation(map[string][]string{"_base": {err.Error()}})
		}
	}
	if err := e.validator.StructCtx(ctx, value); err != nil {
		var invalid *playground.InvalidValidationError
		if errors.As(err, &invalid) {
			return soroerrors.Internal(fmt.Errorf("validate %T: %w", value, err))
		}
		fields := make(map[string][]string)
		var validationErrors playground.ValidationErrors
		if !errors.As(err, &validationErrors) {
			return soroerrors.Internal(err)
		}
		for _, fieldError := range validationErrors {
			field := fieldError.Field()
			fields[field] = append(fields[field], messageFor(fieldError))
		}
		return soroerrors.Validation(fields)
	}
	return nil
}

func messageFor(field playground.FieldError) string {
	switch field.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "max":
		return "must be at most " + field.Param()
	case "min":
		return "must be at least " + field.Param()
	case "uuid", "uuid4", "uuid5":
		return "must be a valid UUID"
	default:
		return "is invalid (" + field.Tag() + ")"
	}
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
