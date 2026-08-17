package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/ruby-dev/soro/api"
	soroerrors "github.com/ruby-dev/soro/errors"
)

type clientContextKey struct{}

type audienceAuthorizer struct {
	events    []string
	scopeErr  error
	clientErr error
}

func (authorizer *audienceAuthorizer) RequireScopes(_ context.Context, audience api.Audience, scopes []string) error {
	authorizer.events = append(authorizer.events, "scopes:"+string(audience)+":"+scopes[0])
	return authorizer.scopeErr
}

func (authorizer *audienceAuthorizer) AuthenticateClient(ctx context.Context, request *http.Request, clientAudience string) (context.Context, error) {
	authorizer.events = append(authorizer.events, "client:"+clientAudience+":"+request.Header.Get("X-Client-Key"))
	if authorizer.clientErr != nil {
		return nil, authorizer.clientErr
	}
	return context.WithValue(ctx, clientContextKey{}, "trusted-client"), nil
}

type audienceOutput struct {
	Body struct {
		Client string `json:"client"`
	}
}

func registerAudienceRoute(t *testing.T, framework *api.API, policy api.AudiencePolicy, called *bool) error {
	t.Helper()
	var registerErr error
	if err := framework.Version("v1", func(router *api.Router) {
		operation := huma.Operation{Method: http.MethodGet, Path: "/audience", OperationID: "get-audience"}
		api.WithAudience(policy)(&operation)
		registerErr = api.Register(router, operation, func(ctx context.Context, _ *struct{}) (*audienceOutput, error) {
			*called = true
			output := &audienceOutput{}
			output.Body.Client, _ = ctx.Value(clientContextKey{}).(string)
			return output, nil
		})
	}); err != nil {
		return err
	}
	return registerErr
}

func TestFirstPartyAudienceRequiresScopesThenIndependentClient(t *testing.T) {
	authorizer := &audienceAuthorizer{}
	framework, err := api.New(api.DefaultConfig(), api.WithAudienceAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	policy := api.FirstParty("control_plane", "global.tenants.admin")
	if err := registerAudienceRoute(t, framework, policy, &called); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/audience", nil)
	request.Header.Set("X-Client-Key", "key-1")
	recorder := httptest.NewRecorder()
	framework.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !called {
		t.Fatalf("response = %d %s, called = %v", recorder.Code, recorder.Body.String(), called)
	}
	wantEvents := []string{"scopes:first_party:global.tenants.admin", "client:control_plane:key-1"}
	if !reflect.DeepEqual(authorizer.events, wantEvents) {
		t.Fatalf("events = %v, want %v", authorizer.events, wantEvents)
	}
	if got := recorder.Header().Get(api.AudienceHeader); got != string(api.AudienceFirstParty) {
		t.Fatalf("audience header = %q", got)
	}
	routes := framework.Routes()
	if len(routes) != 1 || routes[0].Audience != api.AudienceFirstParty || routes[0].ClientAudience != "control_plane" || !reflect.DeepEqual(routes[0].RequiredScopes, []string{"global.tenants.admin"}) {
		t.Fatalf("routes = %+v", routes)
	}
	operation := framework.OpenAPI().Paths["/api/v1/audience"].Get
	if operation.Extensions[api.AudienceExtension] != "first_party" || operation.Extensions[api.AudienceClientAudienceExtension] != "control_plane" {
		t.Fatalf("audience extensions = %+v", operation.Extensions)
	}
}

func TestFirstPartyAudienceStopsBeforeClientWhenNormalAuthorizationFails(t *testing.T) {
	authorizer := &audienceAuthorizer{scopeErr: soroerrors.Forbidden("scope denied")}
	framework, err := api.New(api.DefaultConfig(), api.WithAudienceAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := registerAudienceRoute(t, framework, api.FirstParty("control_plane", "global.tenants.admin"), &called); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audience", nil))
	if recorder.Code != http.StatusForbidden || called {
		t.Fatalf("response = %d %s, called = %v", recorder.Code, recorder.Body.String(), called)
	}
	if len(authorizer.events) != 1 || authorizer.events[0] != "scopes:first_party:global.tenants.admin" {
		t.Fatalf("events = %v", authorizer.events)
	}
}

func TestFirstPartyAudienceRejectsInvalidClientAfterScopeAuthorization(t *testing.T) {
	authorizer := &audienceAuthorizer{clientErr: soroerrors.Unauthorized("client proof required")}
	framework, err := api.New(api.DefaultConfig(), api.WithAudienceAuthorizer(authorizer))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	if err := registerAudienceRoute(t, framework, api.FirstParty("control_plane", "global.tenants.admin"), &called); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audience", nil))
	if recorder.Code != http.StatusUnauthorized || called || len(authorizer.events) != 2 {
		t.Fatalf("response = %d %s, called = %v, events = %v", recorder.Code, recorder.Body.String(), called, authorizer.events)
	}
}

func TestSecondAndThirdPartyAudienceBehavior(t *testing.T) {
	t.Run("second party requires elevated scope without client proof", func(t *testing.T) {
		authorizer := &audienceAuthorizer{}
		framework, err := api.New(api.DefaultConfig(), api.WithAudienceAuthorizer(authorizer))
		if err != nil {
			t.Fatal(err)
		}
		called := false
		if err := registerAudienceRoute(t, framework, api.SecondParty("trusted.reports.read"), &called); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audience", nil))
		if recorder.Code != http.StatusOK || !called || !reflect.DeepEqual(authorizer.events, []string{"scopes:second_party:trusted.reports.read"}) {
			t.Fatalf("response = %d %s, called = %v, events = %v", recorder.Code, recorder.Body.String(), called, authorizer.events)
		}
	})

	t.Run("third party may be explicitly public", func(t *testing.T) {
		framework, err := api.New(api.DefaultConfig())
		if err != nil {
			t.Fatal(err)
		}
		called := false
		if err := registerAudienceRoute(t, framework, api.ThirdParty(), &called); err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		framework.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/audience", nil))
		if recorder.Code != http.StatusOK || !called || recorder.Header().Get(api.AudienceHeader) != "third_party" {
			t.Fatalf("response = %d %s, called = %v", recorder.Code, recorder.Body.String(), called)
		}
	})
}

func TestProtectedAudienceFailsRegistrationWithoutAuthorizer(t *testing.T) {
	framework, err := api.New(api.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = registerAudienceRoute(t, framework, api.SecondParty("trusted.reports.read"), &called)
	if err == nil {
		t.Fatal("expected missing audience authorizer error")
	}
}

func TestUnclassifiedOperationFailsRegistration(t *testing.T) {
	framework, err := api.New(api.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	var registerErr error
	if err := framework.Version("v1", func(router *api.Router) {
		registerErr = api.Register(router, huma.Operation{
			Method: http.MethodGet, Path: "/unclassified", OperationID: "get-unclassified",
		}, func(context.Context, *struct{}) (*audienceOutput, error) {
			return &audienceOutput{}, nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if registerErr == nil {
		t.Fatal("expected unclassified operation error")
	}
}

func TestAudiencePolicyValidation(t *testing.T) {
	for name, policy := range map[string]api.AudiencePolicy{
		"unknown audience":     {Audience: "partner"},
		"first without scope":  {Audience: api.AudienceFirstParty, ClientAudience: "control_plane"},
		"first without client": {Audience: api.AudienceFirstParty, RequiredScopes: []string{"global.read"}},
		"second without scope": {Audience: api.AudienceSecondParty},
		"duplicate scope":      api.ThirdParty("users.read", "users.read"),
	} {
		t.Run(name, func(t *testing.T) {
			if err := policy.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
