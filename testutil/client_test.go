package sorotest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRequestAndDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", request.Method, request.Header)
		}
		writer.Header().Set("X-Test", "true")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	client := &Client{baseURL: server.URL, http: server.Client()}
	response, err := client.Request(t.Context(), http.MethodPost, "/users", map[string]string{"name": "Dustin"})
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		OK bool `json:"ok"`
	}
	if err := response.Decode(&body); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated || response.Header.Get("X-Test") != "true" || !body.OK {
		t.Fatalf("unexpected response: %#v body=%#v", response, body)
	}
	if client.HTTP() == nil {
		t.Fatal("expected underlying HTTP client")
	}
	if resolved, err := client.URL("/health"); err != nil || resolved != server.URL+"/health" {
		t.Fatalf("URL() = %q, %v", resolved, err)
	}
}

func TestClientRejectsUnsafePathsAndInvalidState(t *testing.T) {
	client := &Client{baseURL: "http://example.test", http: http.DefaultClient}
	for _, path := range []string{"users", "//external.example/users"} {
		if _, err := client.Request(context.Background(), http.MethodGet, path, nil); err == nil {
			t.Fatalf("expected path %q to fail", path)
		}
		if _, err := client.URL(path); err == nil {
			t.Fatalf("expected URL path %q to fail", path)
		}
	}
	var missing *Client
	if _, err := missing.Request(t.Context(), http.MethodGet, "/health", nil); err == nil {
		t.Fatal("expected uninitialized client error")
	}
	var response *Response
	if err := response.Decode(&struct{}{}); err == nil {
		t.Fatal("expected nil response error")
	}
}
