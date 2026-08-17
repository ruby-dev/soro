// Package health provides cheap liveness and dependency readiness endpoints.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
)

type Check func(context.Context) error

type Registry struct {
	mu      sync.RWMutex
	checks  map[string]Check
	timeout time.Duration
}

func New(timeout time.Duration) (*Registry, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("health readiness timeout must be positive")
	}
	return &Registry{checks: make(map[string]Check), timeout: timeout}, nil
}

func (registry *Registry) Register(name string, check Check) error {
	if name == "" || check == nil {
		return fmt.Errorf("health check name and function are required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.checks[name]; exists {
		return fmt.Errorf("health check %q is already registered", name)
	}
	registry.checks[name] = check
	return nil
}

type response struct {
	Status string                 `json:"status"`
	Checks map[string]checkStatus `json:"checks,omitempty"`
}

type checkStatus struct {
	Status string `json:"status"`
}

func (registry *Registry) HealthHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, response{Status: "ok"})
	})
}

func (registry *Registry) ReadyHandler() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, cancel := context.WithTimeout(request.Context(), registry.timeout)
		defer cancel()
		checks := registry.snapshot()
		result := response{Status: "ready", Checks: make(map[string]checkStatus, len(checks))}
		status := http.StatusOK
		for _, named := range checks {
			if err := named.check(ctx); err != nil {
				result.Status = "not_ready"
				result.Checks[named.name] = checkStatus{Status: "failed"}
				status = http.StatusServiceUnavailable
			} else {
				result.Checks[named.name] = checkStatus{Status: "ok"}
			}
		}
		writeJSON(writer, status, result)
	})
}

type namedCheck struct {
	name  string
	check Check
}

func (registry *Registry) snapshot() []namedCheck {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	result := make([]namedCheck, 0, len(registry.checks))
	for name, check := range registry.checks {
		result = append(result, namedCheck{name: name, check: check})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].name < result[right].name })
	return result
}

func writeJSON(writer http.ResponseWriter, status int, body response) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(body)
}
