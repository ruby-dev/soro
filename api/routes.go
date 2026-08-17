package api

import (
	"context"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"
)

type Route struct {
	Method      string
	Path        string
	OperationID string
	Tags        []string
}

func (api *API) Routes() []Route {
	api.routesMu.RLock()
	defer api.routesMu.RUnlock()
	routes := make([]Route, len(api.routes))
	for index, route := range api.routes {
		routes[index] = route
		routes[index].Tags = slices.Clone(route.Tags)
	}
	return routes
}

func Register[I, O any](router *Router, operation huma.Operation, handler func(context.Context, *I) (*O, error)) {
	if operation.Method == "" {
		operation.Method = http.MethodGet
	}
	if operation.MaxBodyBytes == 0 {
		operation.MaxBodyBytes = router.owner.config.MaxBodyBytes
	}
	huma.Register(router.huma, operation, func(ctx context.Context, input *I) (*O, error) {
		output, err := handler(ctx, input)
		if err != nil {
			return nil, HTTPError(ctx, err)
		}
		return output, nil
	})
	router.owner.addRoute(Route{
		Method: operation.Method, Path: router.prefix + operation.Path,
		OperationID: operation.OperationID, Tags: slices.Clone(operation.Tags),
	})
}

func (api *API) addRoute(route Route) {
	api.routesMu.Lock()
	defer api.routesMu.Unlock()
	api.routes = append(api.routes, route)
}
