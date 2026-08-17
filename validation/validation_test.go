package validation_test

import (
	"context"
	"testing"

	soroerrors "github.com/ruby-dev/soro/errors"
	"github.com/ruby-dev/soro/validation"
)

func TestDeclarativeValidationNormalizesFields(t *testing.T) {
	input := struct {
		Email string `json:"email" validate:"required,email"`
	}{}
	err := validation.New().Validate(context.Background(), input)
	if !soroerrors.IsCode(err, soroerrors.CodeValidation) {
		t.Fatalf("expected validation error, got %v", err)
	}
	frameworkError := err.(*soroerrors.Error)
	if got := frameworkError.Fields["email"]; len(got) != 1 || got[0] != "is required" {
		t.Fatalf("email errors = %v", got)
	}
}

type contextual struct{ called *bool }

func (v contextual) Validate(context.Context) error {
	*v.called = true
	return nil
}

func TestContextualValidatorRuns(t *testing.T) {
	called := false
	if err := validation.New().Validate(context.Background(), contextual{called: &called}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("contextual validator did not run")
	}
}
