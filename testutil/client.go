package sorotest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// Request sends a request to the in-process Soro application. Input is encoded
// as JSON when non-nil. Paths must be local absolute paths such as /api/v1/users.
func (client *Client) Request(ctx context.Context, method, path string, input any) (*Response, error) {
	if client == nil || client.http == nil || client.baseURL == "" {
		return nil, fmt.Errorf("sorotest client is not initialized")
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, fmt.Errorf("sorotest request path must be a local absolute path")
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("encode sorotest request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create sorotest request: %w", err)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("perform sorotest request: %w", err)
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("read sorotest response: %w", err)
	}
	return &Response{StatusCode: response.StatusCode, Header: response.Header.Clone(), Body: contents}, nil
}

func (response *Response) Decode(destination any) error {
	if response == nil {
		return fmt.Errorf("sorotest response is nil")
	}
	if destination == nil {
		return fmt.Errorf("sorotest response destination is required")
	}
	if err := json.Unmarshal(response.Body, destination); err != nil {
		return fmt.Errorf("decode sorotest response: %w", err)
	}
	return nil
}

func (client *Client) HTTP() *http.Client {
	if client == nil {
		return nil
	}
	return client.http
}

func (client *Client) URL(path string) (string, error) {
	if client == nil || client.baseURL == "" {
		return "", fmt.Errorf("sorotest client is not initialized")
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("sorotest path must be a local absolute path")
	}
	return client.baseURL + path, nil
}
