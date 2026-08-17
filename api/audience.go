package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Audience describes who may build a client for an operation. It is additive
// to normal authentication, authorization, tenant isolation, and auditing.
type Audience string

const (
	AudienceFirstParty  Audience = "first_party"
	AudienceSecondParty Audience = "second_party"
	AudienceThirdParty  Audience = "third_party"
)

const (
	AudienceHeader                  = "X-Soro-API-Audience"
	AudienceExtension               = "x-soro-audience"
	AudienceScopesExtension         = "x-soro-required-scopes"
	AudienceClientAudienceExtension = "x-soro-client-audience"
	audienceMetadataKey             = "soro.audience.policy"
)

// AudiencePolicy is the complete audience requirement for one operation.
// ClientAudience is required only for first-party operations.
type AudiencePolicy struct {
	Audience       Audience
	RequiredScopes []string
	ClientAudience string
}

// FirstParty requires both normal scoped principal authorization and an
// independent software-client credential for clientAudience.
func FirstParty(clientAudience string, requiredScope string, additionalScopes ...string) AudiencePolicy {
	return AudiencePolicy{
		Audience:       AudienceFirstParty,
		ClientAudience: clientAudience,
		RequiredScopes: append([]string{requiredScope}, additionalScopes...),
	}.normalized()
}

// SecondParty requires one or more elevated scopes granted to vetted clients.
func SecondParty(requiredScope string, additionalScopes ...string) AudiencePolicy {
	return AudiencePolicy{
		Audience:       AudienceSecondParty,
		RequiredScopes: append([]string{requiredScope}, additionalScopes...),
	}.normalized()
}

// ThirdParty describes the public developer surface. An empty scope list is
// explicit public/anonymous access; otherwise normal scope authorization runs.
func ThirdParty(requiredScopes ...string) AudiencePolicy {
	return AudiencePolicy{Audience: AudienceThirdParty, RequiredScopes: slices.Clone(requiredScopes)}.normalized()
}

func (policy AudiencePolicy) normalized() AudiencePolicy {
	if policy.Audience == "" {
		policy.Audience = AudienceThirdParty
	}
	policy.ClientAudience = strings.TrimSpace(policy.ClientAudience)
	policy.RequiredScopes = slices.Clone(policy.RequiredScopes)
	for index := range policy.RequiredScopes {
		policy.RequiredScopes[index] = strings.TrimSpace(policy.RequiredScopes[index])
	}
	return policy
}

func (policy AudiencePolicy) isZero() bool {
	return policy.Audience == "" && policy.ClientAudience == "" && len(policy.RequiredScopes) == 0
}

func (policy AudiencePolicy) Validate() error {
	policy = policy.normalized()
	switch policy.Audience {
	case AudienceFirstParty, AudienceSecondParty, AudienceThirdParty:
	default:
		return fmt.Errorf("unsupported API audience %q", policy.Audience)
	}
	seen := make(map[string]struct{}, len(policy.RequiredScopes))
	for _, scope := range policy.RequiredScopes {
		if scope == "" {
			return fmt.Errorf("API audience scope cannot be empty")
		}
		if _, exists := seen[scope]; exists {
			return fmt.Errorf("API audience scope %q is duplicated", scope)
		}
		seen[scope] = struct{}{}
	}
	if policy.Audience == AudienceFirstParty {
		if policy.ClientAudience == "" {
			return fmt.Errorf("first-party API audience requires a client audience")
		}
		if len(policy.RequiredScopes) == 0 {
			return fmt.Errorf("first-party API audience requires normal authorization scopes")
		}
		return nil
	}
	if policy.ClientAudience != "" {
		return fmt.Errorf("%s API audience cannot declare a client audience", policy.Audience)
	}
	if policy.Audience == AudienceSecondParty && len(policy.RequiredScopes) == 0 {
		return fmt.Errorf("second-party API audience requires an elevated scope")
	}
	return nil
}

// WithAudience marks a Huma operation. Soro validates and enforces the policy
// when the operation is registered.
func WithAudience(policy AudiencePolicy) func(*huma.Operation) {
	return func(operation *huma.Operation) {
		if operation.Metadata == nil {
			operation.Metadata = make(map[string]any)
		}
		operation.Metadata[audienceMetadataKey] = policy.normalized()
	}
}

// AudienceAuthorizer bridges Soro's audience policy to application-owned
// principal scopes and software-client credentials. AuthenticateClient may
// return a derived context containing the authenticated client identity.
type AudienceAuthorizer interface {
	RequireScopes(context.Context, Audience, []string) error
	AuthenticateClient(context.Context, *http.Request, string) (context.Context, error)
}

func audiencePolicy(operation *huma.Operation) (AudiencePolicy, bool) {
	if operation.Metadata != nil {
		if policy, ok := operation.Metadata[audienceMetadataKey].(AudiencePolicy); ok {
			return policy.normalized(), true
		}
	}
	return AudiencePolicy{}, false
}

func (api *API) prepareAudience(operation *huma.Operation) (AudiencePolicy, error) {
	policy, declared := audiencePolicy(operation)
	if !declared {
		return AudiencePolicy{}, fmt.Errorf("operation %q: API audience must be declared", operation.OperationID)
	}
	if err := policy.Validate(); err != nil {
		return AudiencePolicy{}, fmt.Errorf("operation %q: %w", operation.OperationID, err)
	}
	if api.audienceAuthorizer == nil && (len(policy.RequiredScopes) > 0 || policy.Audience == AudienceFirstParty) {
		return AudiencePolicy{}, fmt.Errorf("operation %q: API audience authorizer is required", operation.OperationID)
	}
	if operation.Extensions == nil {
		operation.Extensions = make(map[string]any)
	}
	operation.Extensions[AudienceExtension] = string(policy.Audience)
	if len(policy.RequiredScopes) > 0 {
		operation.Extensions[AudienceScopesExtension] = slices.Clone(policy.RequiredScopes)
	} else {
		delete(operation.Extensions, AudienceScopesExtension)
	}
	if policy.ClientAudience != "" {
		operation.Extensions[AudienceClientAudienceExtension] = policy.ClientAudience
	} else {
		delete(operation.Extensions, AudienceClientAudienceExtension)
	}
	operation.Middlewares = append(huma.Middlewares{api.audienceMiddleware(policy)}, operation.Middlewares...)
	return policy, nil
}

func (api *API) audienceMiddleware(policy AudiencePolicy) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		ctx.SetHeader(AudienceHeader, string(policy.Audience))
		if len(policy.RequiredScopes) > 0 {
			if err := api.audienceAuthorizer.RequireScopes(ctx.Context(), policy.Audience, slices.Clone(policy.RequiredScopes)); err != nil {
				api.writeAudienceError(ctx, err)
				return
			}
		}
		if policy.Audience == AudienceFirstParty {
			request, _ := humago.Unwrap(ctx)
			authorizedContext, err := api.audienceAuthorizer.AuthenticateClient(ctx.Context(), request, policy.ClientAudience)
			if err != nil {
				api.writeAudienceError(ctx, err)
				return
			}
			if authorizedContext == nil {
				api.writeAudienceError(ctx, fmt.Errorf("audience authorizer returned a nil context"))
				return
			}
			ctx = huma.WithContext(ctx, authorizedContext)
		}
		next(ctx)
	}
}

func (api *API) writeAudienceError(ctx huma.Context, err error) {
	converted := HTTPError(ctx.Context(), err)
	var statusError huma.StatusError
	if !errors.As(converted, &statusError) {
		statusError = huma.Error500InternalServerError("Internal server error")
	}
	_ = huma.WriteErr(api.huma, ctx, statusError.GetStatus(), statusError.Error())
}
