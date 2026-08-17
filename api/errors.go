package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	soroerrors "github.com/datasoro/soro/errors"
)

type ErrorEnvelope struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string              `json:"code"`
	Message   string              `json:"message"`
	RequestID string              `json:"request_id,omitempty"`
	Fields    map[string][]string `json:"fields,omitempty"`
}

type StatusError struct {
	ErrorEnvelope
	status int
}

func (response *StatusError) Error() string { return response.ErrorEnvelope.Error.Message }

func (response *StatusError) GetStatus() int { return response.status }

func (response *StatusError) ContentType(contentType string) string {
	return contentType
}

var humaErrorOnce sync.Once

func installHumaErrorFactory() {
	humaErrorOnce.Do(func() {
		huma.NewError = func(status int, message string, details ...error) huma.StatusError {
			return humaError(status, message, "", details...)
		}
		huma.NewErrorWithContext = func(ctx huma.Context, status int, message string, details ...error) huma.StatusError {
			return humaError(status, message, RequestID(ctx.Context()), details...)
		}
	})
}

func HTTPError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var statusError huma.StatusError
	if errors.As(err, &statusError) {
		return err
	}
	var frameworkError *soroerrors.Error
	if errors.As(err, &frameworkError) {
		return &StatusError{
			status: statusForCode(frameworkError.Code),
			ErrorEnvelope: ErrorEnvelope{Error: ErrorBody{
				Code: string(frameworkError.Code), Message: frameworkError.Message,
				RequestID: RequestID(ctx), Fields: cloneFields(frameworkError.Fields),
			}},
		}
	}
	return &StatusError{
		status:        http.StatusInternalServerError,
		ErrorEnvelope: ErrorEnvelope{Error: ErrorBody{Code: string(soroerrors.CodeInternal), Message: "Internal server error", RequestID: RequestID(ctx)}},
	}
}

func humaError(status int, message, requestID string, details ...error) *StatusError {
	code := codeForStatus(status)
	fields := make(map[string][]string)
	for _, detailError := range details {
		if detailError == nil {
			continue
		}
		if detailer, ok := detailError.(huma.ErrorDetailer); ok {
			detail := detailer.ErrorDetail()
			field := normalizeLocation(detail.Location)
			if field == "" {
				field = "_base"
			}
			fields[field] = append(fields[field], detail.Message)
		} else {
			fields["_base"] = append(fields["_base"], detailError.Error())
		}
	}
	if len(fields) == 0 {
		fields = nil
	}
	if status == http.StatusUnprocessableEntity {
		message = "Validation failed"
	}
	if status >= 500 {
		message = "Internal server error"
	}
	return &StatusError{
		status:        status,
		ErrorEnvelope: ErrorEnvelope{Error: ErrorBody{Code: code, Message: message, RequestID: requestID, Fields: fields}},
	}
}

func statusForCode(code soroerrors.Code) int {
	switch code {
	case soroerrors.CodeNotFound:
		return http.StatusNotFound
	case soroerrors.CodeValidation:
		return http.StatusUnprocessableEntity
	case soroerrors.CodeConflict:
		return http.StatusConflict
	case soroerrors.CodeForbidden:
		return http.StatusForbidden
	case soroerrors.CodeUnauthorized:
		return http.StatusUnauthorized
	case soroerrors.CodeInvalidArgument, soroerrors.CodeInvalidQuery:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusUnauthorized:
		return string(soroerrors.CodeUnauthorized)
	case http.StatusForbidden:
		return string(soroerrors.CodeForbidden)
	case http.StatusNotFound:
		return string(soroerrors.CodeNotFound)
	case http.StatusConflict:
		return string(soroerrors.CodeConflict)
	case http.StatusRequestEntityTooLarge:
		return "request_too_large"
	case http.StatusUnprocessableEntity:
		return string(soroerrors.CodeValidation)
	default:
		if status >= 500 {
			return string(soroerrors.CodeInternal)
		}
		return "request_failed"
	}
}

func normalizeLocation(location string) string {
	for _, prefix := range []string{"body.", "query.", "path.", "header."} {
		location = strings.TrimPrefix(location, prefix)
	}
	return location
}

func cloneFields(fields map[string][]string) map[string][]string {
	if len(fields) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(fields))
	for field, messages := range fields {
		cloned[field] = append([]string(nil), messages...)
	}
	return cloned
}
