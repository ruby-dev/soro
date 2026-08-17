package api_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/datasoro/soro/api"
)

func TestVersionedRouteRequestIDAndOpenAPI(t *testing.T) {
	framework, err := api.New(api.DefaultConfig(), api.WithRequestIDGenerator(func() string { return "request-1" }))
	if err != nil {
		t.Fatal(err)
	}
	if err := framework.Version("v1", func(router *api.Router) {
		api.Register(router, huma.Operation{
			Method: http.MethodGet, Path: "/hello", OperationID: "get-hello", Tags: []string{"Hello"},
		}, func(context.Context, *struct{}) (*struct {
			Body struct {
				Message string `json:"message"`
			}
		}, error) {
			output := &struct {
				Body struct {
					Message string `json:"message"`
				}
			}{}
			output.Body.Message = "hello"
			return output, nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/hello", nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get(api.RequestIDHeader) != "request-1" {
		t.Fatalf("response = %d, headers %v", recorder.Code, recorder.Header())
	}
	routes := framework.Routes()
	if len(routes) != 1 || routes[0].Path != "/api/v1/hello" || routes[0].OperationID != "get-hello" {
		t.Fatalf("routes = %+v", routes)
	}
	if framework.OpenAPI().Paths == nil {
		t.Fatal("OpenAPI paths are missing")
	}

	openAPI := httptest.NewRecorder()
	framework.Handler().ServeHTTP(openAPI, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if openAPI.Code != http.StatusOK {
		t.Fatalf("OpenAPI response = %d: %s", openAPI.Code, openAPI.Body.String())
	}
}

func TestRecoveryUsesStandardEnvelope(t *testing.T) {
	framework, err := api.New(api.DefaultConfig(),
		api.WithRequestIDGenerator(func() string { return "panic-request" }),
		api.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	if err != nil {
		t.Fatal(err)
	}
	framework.Mux().HandleFunc("GET /panic", func(http.ResponseWriter, *http.Request) { panic("secret") })
	recorder := httptest.NewRecorder()
	framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	var envelope api.ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "internal_error" || envelope.Error.RequestID != "panic-request" {
		t.Fatalf("error = %+v", envelope.Error)
	}
}

func TestInvalidVersionIsRejected(t *testing.T) {
	framework, err := api.New(api.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := framework.Version("../v1", func(*api.Router) {}); err == nil {
		t.Fatal("expected invalid version error")
	}
}
