package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	soroerrors "github.com/ruby-dev/soro/errors"
)

func TestInternalDoesNotSerializeCause(t *testing.T) {
	cause := stderrors.New("password=secret")
	err := soroerrors.Internal(cause)

	data, marshalErr := json.Marshal(err)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if strings.Contains(string(data), "secret") {
		t.Fatalf("serialized internal cause: %s", data)
	}
	if !stderrors.Is(err, cause) {
		t.Fatal("internal cause is not available through errors.Is")
	}
}

func TestIsCode(t *testing.T) {
	err := soroerrors.NotFound("User")
	if !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		t.Fatal("expected not_found code")
	}
}
