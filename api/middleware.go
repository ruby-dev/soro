package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func defaultRequestID() string { return uuid.NewString() }

func (api *API) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := api.newID()
		writer.Header().Set(RequestIDHeader, requestID)
		ctx := context.WithValue(request.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func (api *API) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestID := RequestID(request.Context())
				api.logger.ErrorContext(request.Context(), "recovered HTTP panic", "request_id", requestID)
				writer.Header().Set("Content-Type", "application/json")
				writer.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(writer).Encode(ErrorEnvelope{
					Error: ErrorBody{Code: "internal_error", Message: "Internal server error", RequestID: requestID},
				})
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
