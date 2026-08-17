package generate

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type NewOptions struct {
	ParentDirectory string
	Name            string
	Module          string
	SoroVersion     string
	SoroReplace     string
	Force           bool
}

func NewApplication(options NewOptions) ([]string, error) {
	name, err := applicationName(options.Name)
	if err != nil {
		return nil, err
	}
	if options.Module == "" {
		options.Module = name
	}
	if err := ValidateModule(options.Module); err != nil {
		return nil, err
	}
	if options.SoroVersion == "" {
		options.SoroVersion = "v0.0.0"
	}
	target := filepath.Join(options.ParentDirectory, name)
	if info, statErr := os.Stat(target); statErr == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("application path %s is not a directory", target)
		}
		entries, readErr := os.ReadDir(target)
		if readErr != nil {
			return nil, readErr
		}
		if len(entries) > 0 && !options.Force {
			return nil, fmt.Errorf("application directory %s is not empty", target)
		}
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}
	moduleFile := fmt.Sprintf("module %s\n\ngo 1.26.0\n\nrequire %s %s\n", options.Module, FrameworkModule, options.SoroVersion)
	if options.SoroReplace != "" {
		replacement, absoluteErr := filepath.Abs(options.SoroReplace)
		if absoluteErr != nil {
			return nil, absoluteErr
		}
		moduleFile += fmt.Sprintf("\nreplace %s => %s\n", FrameworkModule, filepath.ToSlash(replacement))
	}
	files := []generatedFile{
		{path: "go.mod", content: []byte(moduleFile)},
		{path: ".gitignore", content: []byte(".env\n.env.*\n!.env.example\ncoverage.out\n")},
		{path: ".env.example", content: []byte("SORO_ENV=development\nDATABASE_URL=postgres://postgres:postgres@localhost:5432/" + strings.ReplaceAll(name, "-", "_") + "?sslmode=disable\n")},
		{path: filepath.Join("config", "application.yaml"), content: []byte("app:\n  name: " + name + "\nhttp:\n  port: 8080\n  api_base_path: /api\n")},
		{path: filepath.Join("config", "development.yaml"), content: []byte("database:\n  url: postgres://postgres:postgres@localhost:5432/" + strings.ReplaceAll(name, "-", "_") + "?sslmode=disable\nmail:\n  transport: console\n")},
		{path: filepath.Join("config", "test.yaml"), content: []byte("database:\n  url: postgres://postgres:postgres@localhost:5432/" + strings.ReplaceAll(name, "-", "_") + "_test?sslmode=disable\nmail:\n  transport: capture\nobservability:\n  enabled: false\n")},
		{path: filepath.Join("app", "application.go"), content: []byte(applicationSource(options.Module)), goFile: true},
		{path: filepath.Join("app", "api", "v1", "routes.go"), content: mustRenderRoutes(options.Module), goFile: true, managed: true},
		{path: filepath.Join("app", "jobs", "register.go"), content: mustRenderJobs(nil), goFile: true, managed: true},
		{path: filepath.Join("app", "seeds", "seeds.go"), content: []byte(seedSource()), goFile: true},
		{path: filepath.Join("cmd", "server", "main.go"), content: []byte(serverSource(options.Module)), goFile: true},
		{path: filepath.Join("cmd", "app", "main.go"), content: []byte(controlSource(options.Module)), goFile: true},
		{path: filepath.Join("db", "seeds", ".gitkeep"), content: []byte{}},
		{path: filepath.Join("templates", ".gitkeep"), content: []byte{}},
		{path: "README.md", content: []byte(applicationReadme(name))},
	}
	generator := &Generator{Root: target, Module: options.Module, Force: options.Force}
	return generator.writeFiles(files)
}

func applicationName(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return "", fmt.Errorf("invalid application name %q", raw)
	}
	for _, character := range value {
		if character != '-' && character != '_' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return "", fmt.Errorf("application name must contain lowercase letters, digits, hyphens, or underscores")
		}
	}
	if value[0] < 'a' || value[0] > 'z' {
		return "", fmt.Errorf("application name must begin with a letter")
	}
	return value, nil
}

func applicationSource(module string) string {
	return fmt.Sprintf(`package app

import (
	"context"
	"errors"

	"github.com/datasoro/soro"
	v1 %q
	appjobs %q
)

func Build(ctx context.Context) (*soro.App, error) {
	application, err := soro.New(ctx)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(v1.Register(application), appjobs.Register(application.Jobs)); err != nil {
		_ = application.Close()
		return nil, err
	}
	return application, nil
}
`, module+"/app/api/v1", module+"/app/jobs")
}

func serverSource(module string) string {
	return fmt.Sprintf(`package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	application %q
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := application.Build(ctx)
	if err != nil { log.Fatal(err) }
	defer app.Close()
	if err := app.Serve(ctx); err != nil { log.Fatal(err) }
}
`, module+"/app")
}

func controlSource(module string) string {
	return fmt.Sprintf(`package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"

	application %q
	%q
)

func main() {
	if len(os.Args) != 2 { log.Fatal("usage: app routes|jobs|openapi|seed") }
	if os.Args[1] == "jobs" { _ = os.Setenv("SORO_JOBS_ENABLED", "true") }
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app, err := application.Build(ctx)
	if err != nil { log.Fatal(err) }
	defer app.Close()
	switch os.Args[1] {
	case "routes":
		routes := app.API.Routes()
		sort.Slice(routes, func(i, j int) bool { return routes[i].Path < routes[j].Path || routes[i].Path == routes[j].Path && routes[i].Method < routes[j].Method })
		for _, route := range routes { fmt.Printf("%%-7s %%s\t%%s\n", route.Method, route.Path, route.OperationID) }
	case "jobs":
		if err := app.Jobs.Start(ctx); err != nil { log.Fatal(err) }
		<-ctx.Done()
	case "openapi":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(app.API.OpenAPI()); err != nil { log.Fatal(err) }
	case "seed":
		if err := seeds.Run(ctx, app); err != nil { log.Fatal(err) }
	default:
		log.Fatalf("unknown app command %%q", os.Args[1])
	}
}
`, module+"/app", module+"/app/seeds")
}

func seedSource() string {
	return `package seeds

import (
	"context"

	"github.com/datasoro/soro"
)

// Run inserts development or test seed data. Keep seed operations explicit and idempotent where practical.
func Run(ctx context.Context, app *soro.App) error {
	_ = ctx
	_ = app
	return nil
}
`
}

func applicationReadme(name string) string {
	return fmt.Sprintf("# %s\n\nGenerated by Soro.\n\n```sh\ncp .env.example .env\nsoro db create\nsoro db migrate\nsoro server\n```\n", name)
}

func mustRenderRoutes(module string) []byte {
	contents, err := renderRoutes(module, nil)
	if err != nil {
		panic(err)
	}
	return contents
}

func mustRenderJobs(names []Name) []byte {
	contents, err := renderJobRegistry(names)
	if err != nil {
		panic(err)
	}
	return contents
}

func renderJobRegistry(names []Name) ([]byte, error) {
	var source strings.Builder
	source.WriteString("// Code generated by Soro. DO NOT EDIT.\npackage jobs\n\n")
	source.WriteString("import sorojobs \"github.com/datasoro/soro/jobs\"\n\n")
	source.WriteString("func Register(client *sorojobs.Client) error {\n")
	for _, name := range names {
		fmt.Fprintf(&source, "\tif err := sorojobs.Register(client, Perform%s); err != nil { return err }\n", name.Singular)
	}
	source.WriteString("\treturn nil\n}\n")
	return formatted(source.String())
}

func (generator *Generator) jobRegistrations(current Name) ([]Name, error) {
	directory := filepath.Join(generator.Root, "app", "jobs")
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	byName := map[string]Name{current.Singular: current}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "register.go" || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		for _, line := range strings.Split(string(contents), "\n") {
			const marker = "// soro:job "
			if strings.HasPrefix(line, marker) {
				parsed, parseErr := ParseName(strings.TrimPrefix(line, marker))
				if parseErr != nil {
					return nil, parseErr
				}
				byName[parsed.Singular] = parsed
			}
		}
	}
	result := make([]Name, 0, len(byName))
	for _, name := range byName {
		result = append(result, name)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Singular < result[j].Singular })
	return result, nil
}
