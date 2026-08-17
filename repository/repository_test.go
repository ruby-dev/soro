package repository_test

import (
	"context"
	"testing"

	soroerrors "github.com/ruby-dev/soro/errors"
	"github.com/ruby-dev/soro/repository"
)

func TestInvalidModelContractIsReported(t *testing.T) {
	type invalid struct{ ID int }
	repo := repository.New[invalid](nil)
	_, err := repo.Find(context.Background(), [16]byte{1})
	if !soroerrors.IsCode(err, soroerrors.CodeInvalidArgument) {
		t.Fatalf("expected invalid argument, got %v", err)
	}
}
