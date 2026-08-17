package sorotest_test

import (
	"net/http"
	"testing"

	"github.com/datasoro/soro/testutil"
)

func BenchmarkHealthRequest(b *testing.B) {
	app := sorotest.New(b)
	b.ResetTimer()
	for b.Loop() {
		response, err := app.Client().Request(b.Context(), http.MethodGet, "/health", nil)
		if err != nil || response.StatusCode != http.StatusOK {
			b.Fatalf("health status=%v err=%v", response, err)
		}
	}
}
