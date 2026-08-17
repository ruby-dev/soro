package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/migrate"
	"github.com/datasoro/soro/repository"
)

func TestResourceCRUDQueryErrorsAndOpenAPI(t *testing.T) {
	db := testdb.Open(t)
	if err := migrate.New(db).Apply(t.Context(), basic.Migrations); err != nil {
		t.Fatal(err)
	}
	httpAPI, err := api.New(api.DefaultConfig(), api.WithRequestIDGenerator(func() string { return "request-test" }))
	if err != nil {
		t.Fatal(err)
	}
	resource, err := basic.NewUserResource(repository.New[basic.User](db))
	if err != nil {
		t.Fatal(err)
	}
	if err := httpAPI.Version("v1", func(v1 *api.Router) {
		if routeErr := v1.Resource("/users", resource); routeErr != nil {
			t.Fatal(routeErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpAPI.Handler())
	t.Cleanup(server.Close)

	invalid := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/users", `{"name":"Bad","email":"not-an-email","active":true}`)
	if invalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("validation status = %d, body = %s", invalid.StatusCode, readBody(t, invalid))
	}
	invalidBody := decodeBody(t, invalid)
	errorBody := invalidBody["error"].(map[string]any)
	if errorBody["code"] != "validation_failed" || errorBody["request_id"] != "request-test" {
		t.Fatalf("unexpected validation error: %#v", errorBody)
	}

	dustin := createUser(t, server, `{"name":"Dustin","email":"DUSTIN@EXAMPLE.COM","active":true}`)
	ward := createUser(t, server, `{"name":"Ward","email":"ward@example.com","active":false}`)
	if dustin["metadata"] != nil || dustin["deleted_at"] != nil {
		t.Fatalf("serializer exposed persistence fields: %#v", dustin)
	}
	if dustin["email"] != "dustin@example.com" {
		t.Fatalf("create lifecycle hook did not normalize email: %#v", dustin)
	}

	list := get(t, server, "/api/v1/users?filter[active]=true&search=dus&sort=-created_at&page=1&per_page=1")
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", list.StatusCode, readBody(t, list))
	}
	listBody := decodeBody(t, list)
	data := listBody["data"].([]any)
	meta := listBody["meta"].(map[string]any)
	if len(data) != 1 || meta["total"] != float64(1) || meta["pages"] != float64(1) {
		t.Fatalf("unexpected list response: %#v", listBody)
	}

	wardID := ward["id"].(string)
	patch := requestJSON(t, server.Client(), http.MethodPatch, server.URL+"/api/v1/users/"+wardID, `{"active":true,"name":"Ward Updated"}`)
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, body = %s", patch.StatusCode, readBody(t, patch))
	}
	patched := decodeBody(t, patch)["data"].(map[string]any)
	if patched["active"] != true || patched["name"] != "Ward Updated" {
		t.Fatalf("unexpected patch response: %#v", patched)
	}

	deleted := requestJSON(t, server.Client(), http.MethodDelete, server.URL+"/api/v1/users/"+wardID, "")
	if deleted.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleted.StatusCode, readBody(t, deleted))
	}
	missing := get(t, server, "/api/v1/users/"+wardID)
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("soft-deleted show status = %d, body = %s", missing.StatusCode, readBody(t, missing))
	}

	unsafe := get(t, server, "/api/v1/users?sort=password")
	if unsafe.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsafe sort status = %d, body = %s", unsafe.StatusCode, readBody(t, unsafe))
	}
	if decodeBody(t, unsafe)["error"].(map[string]any)["code"] != "invalid_query" {
		t.Fatalf("unexpected unsafe query response")
	}

	openAPI := get(t, server, "/openapi.json")
	if openAPI.StatusCode != http.StatusOK {
		t.Fatalf("OpenAPI status = %d", openAPI.StatusCode)
	}
	spec := readBody(t, openAPI)
	for _, expected := range []string{`"/api/v1/users"`, `"operationId":"list-users"`, `"filter[active]"`} {
		if !strings.Contains(spec, expected) {
			t.Fatalf("OpenAPI does not contain %s", expected)
		}
	}
	if got := len(httpAPI.Routes()); got != 5 {
		t.Fatalf("registered routes = %d, want 5", got)
	}
}

func createUser(t *testing.T, server *httptest.Server, body string) map[string]any {
	t.Helper()
	response := requestJSON(t, server.Client(), http.MethodPost, server.URL+"/api/v1/users", body)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.StatusCode, readBody(t, response))
	}
	if response.Header.Get("X-Request-ID") != "request-test" {
		t.Fatalf("missing request ID header")
	}
	return decodeBody(t, response)["data"].(map[string]any)
}

func get(t *testing.T, server *httptest.Server, path string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func requestJSON(t *testing.T, client *http.Client, method, target, body string) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), method, target, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeBody(t *testing.T, response *http.Response) map[string]any {
	t.Helper()
	defer response.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
