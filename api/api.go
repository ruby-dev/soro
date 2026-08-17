// Package api wraps Huma with Soro routing, errors, resources, and middleware.
package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

type Config struct {
	Title        string
	Version      string
	BasePath     string
	OpenAPIPath  string
	DocsPath     string
	SchemasPath  string
	MaxBodyBytes int64
}

func DefaultConfig() Config {
	return Config{
		Title: "Soro API", Version: "0.0.0", BasePath: "/api",
		OpenAPIPath: "/openapi", DocsPath: "/docs", SchemasPath: "/schemas",
		MaxBodyBytes: 1024 * 1024,
	}
}

func (config Config) Validate() error {
	if strings.TrimSpace(config.Title) == "" || strings.TrimSpace(config.Version) == "" {
		return fmt.Errorf("api title and version are required")
	}
	for name, configured := range map[string]string{
		"base path": config.BasePath, "OpenAPI path": config.OpenAPIPath,
		"docs path": config.DocsPath, "schemas path": config.SchemasPath,
	} {
		if configured != "" && (!strings.HasPrefix(configured, "/") || (len(configured) > 1 && strings.HasSuffix(configured, "/"))) {
			return fmt.Errorf("api %s must begin with / and must not end with /", name)
		}
	}
	if config.MaxBodyBytes < 1 {
		return fmt.Errorf("api max body bytes must be positive")
	}
	return nil
}

type settings struct {
	logger             *slog.Logger
	requestIDGenerator func() string
	middleware         []Middleware
}

type Middleware func(http.Handler) http.Handler

type Option func(*settings)

func WithLogger(logger *slog.Logger) Option {
	return func(settings *settings) { settings.logger = logger }
}

func WithRequestIDGenerator(generator func() string) Option {
	return func(settings *settings) { settings.requestIDGenerator = generator }
}

func WithMiddleware(middleware ...Middleware) Option {
	return func(settings *settings) { settings.middleware = append(settings.middleware, middleware...) }
}

type API struct {
	config     Config
	mux        *http.ServeMux
	huma       huma.API
	logger     *slog.Logger
	newID      func() string
	middleware []Middleware

	routesMu sync.RWMutex
	routes   []Route
}

func New(config Config, options ...Option) (*API, error) {
	defaults := DefaultConfig()
	if config.Title == "" {
		config.Title = defaults.Title
	}
	if config.Version == "" {
		config.Version = defaults.Version
	}
	if config.BasePath == "" {
		config.BasePath = defaults.BasePath
	}
	if config.OpenAPIPath == "" {
		config.OpenAPIPath = defaults.OpenAPIPath
	}
	if config.DocsPath == "" {
		config.DocsPath = defaults.DocsPath
	}
	if config.SchemasPath == "" {
		config.SchemasPath = defaults.SchemasPath
	}
	if config.MaxBodyBytes == 0 {
		config.MaxBodyBytes = defaults.MaxBodyBytes
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	configured := settings{logger: slog.Default(), requestIDGenerator: defaultRequestID}
	for _, option := range options {
		option(&configured)
	}
	if configured.logger == nil || configured.requestIDGenerator == nil {
		return nil, fmt.Errorf("api logger and request ID generator are required")
	}
	installHumaErrorFactory()
	mux := http.NewServeMux()
	humaConfig := huma.DefaultConfig(config.Title, config.Version)
	humaConfig.OpenAPIPath = config.OpenAPIPath
	humaConfig.DocsPath = config.DocsPath
	humaConfig.SchemasPath = config.SchemasPath
	humaConfig.DocsRenderer = huma.DocsRendererScalar
	humaAPI := humago.New(mux, humaConfig)
	for _, middleware := range configured.middleware {
		if middleware == nil {
			return nil, fmt.Errorf("api middleware cannot be nil")
		}
	}
	return &API{config: config, mux: mux, huma: humaAPI, logger: configured.logger, newID: configured.requestIDGenerator, middleware: configured.middleware}, nil
}

func (api *API) Config() Config { return api.config }

func (api *API) Huma() huma.API { return api.huma }

func (api *API) Mux() *http.ServeMux { return api.mux }

func (api *API) OpenAPI() *huma.OpenAPI { return api.huma.OpenAPI() }

// Handler returns the complete HTTP middleware chain.
func (api *API) Handler() http.Handler {
	var handler http.Handler = api.recoveryMiddleware(api.mux)
	for index := len(api.middleware) - 1; index >= 0; index-- {
		handler = api.middleware[index](handler)
	}
	return api.requestIDMiddleware(handler)
}

var versionPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

func (api *API) Version(name string, configure func(*Router)) error {
	if !versionPattern.MatchString(name) {
		return fmt.Errorf("invalid API version %q", name)
	}
	if configure == nil {
		return fmt.Errorf("API version callback is required")
	}
	prefix := strings.TrimSuffix(api.config.BasePath, "/") + "/" + name
	router := &Router{owner: api, huma: huma.NewGroup(api.huma, prefix), prefix: prefix}
	configure(router)
	return nil
}

type Router struct {
	owner  *API
	huma   huma.API
	prefix string
}

func (router *Router) Huma() huma.API { return router.huma }

func (router *Router) Prefix() string { return router.prefix }

type Registrar interface {
	Register(*Router, string)
}

func (router *Router) Resource(path string, resource Registrar) error {
	if resource == nil {
		return fmt.Errorf("resource is required")
	}
	if !validRoutePath(path) {
		return fmt.Errorf("invalid resource path %q", path)
	}
	resource.Register(router, path)
	return nil
}

func validRoutePath(path string) bool {
	return strings.HasPrefix(path, "/") && path != "/" && !strings.HasSuffix(path, "/") && !strings.Contains(path, "//")
}
