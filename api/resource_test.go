package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruby-dev/soro/api"
	soroerrors "github.com/ruby-dev/soro/errors"
	"github.com/ruby-dev/soro/examples/basic"
	"github.com/ruby-dev/soro/repository"
	"github.com/ruby-dev/soro/serializer"
)

func TestResourceDisabledOperationsAndAuthorization(t *testing.T) {
	resource, err := api.NewResource(api.ResourceConfig[basic.User, struct{}, struct{}, struct{}]{
		Name:       "Locked Users",
		Repository: repository.New[basic.User](nil),
		Serializer: serializer.Func[basic.User, struct{}](func(context.Context, *basic.User) (struct{}, error) {
			return struct{}{}, nil
		}),
		Disabled: []api.Action{api.Create, api.Update, api.Destroy},
		Authorize: func(context.Context, api.Action, *basic.User) error {
			return soroerrors.Forbidden("Access denied")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	framework, err := api.New(api.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := framework.Version("v1", func(v1 *api.Router) {
		if routeErr := v1.Resource("/locked-users", resource); routeErr != nil {
			t.Fatal(routeErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if routes := framework.Routes(); len(routes) != 2 {
		t.Fatalf("routes = %+v, want index and show", routes)
	}

	recorder := httptest.NewRecorder()
	framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/locked-users", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	destroy := httptest.NewRecorder()
	framework.Handler().ServeHTTP(destroy, httptest.NewRequest(http.MethodDelete, "/api/v1/locked-users/00000000-0000-0000-0000-000000000001", nil))
	if destroy.Code != http.StatusMethodNotAllowed {
		t.Fatalf("disabled destroy status = %d", destroy.Code)
	}
}
