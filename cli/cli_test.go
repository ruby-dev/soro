package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type runCall struct {
	directory string
	name      string
	arguments []string
}

type fakeRunner struct {
	calls  []runCall
	output string
	err    error
}

func (runner *fakeRunner) Run(_ context.Context, directory string, _ []string, _ io.Reader, stdout, _ io.Writer, name string, arguments ...string) error {
	runner.calls = append(runner.calls, runCall{directory: directory, name: name, arguments: slices.Clone(arguments)})
	_, _ = io.WriteString(stdout, runner.output)
	return runner.err
}

func TestCommandTreeAndVersion(t *testing.T) {
	var output bytes.Buffer
	command := New(Settings{Directory: t.TempDir(), Stdout: &output, Stderr: &output, Version: "1.2.3"})
	command.SetArgs([]string{"version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "soro 1.2.3\n" {
		t.Fatalf("unexpected version output %q", output.String())
	}

	output.Reset()
	command = New(Settings{Directory: t.TempDir(), Stdout: &output, Stderr: &output})
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"new", "server", "routes", "generate", "db", "jobs", "openapi", "version"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("help does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestApplicationDelegatingCommands(t *testing.T) {
	runner := &fakeRunner{}
	var output bytes.Buffer
	for _, arguments := range [][]string{{"server"}, {"routes"}, {"jobs", "work"}, {"db", "seed"}} {
		command := New(Settings{Directory: "/application", Stdout: &output, Stderr: &output, Runner: runner})
		command.SetArgs(arguments)
		if err := command.Execute(); err != nil {
			t.Fatalf("execute %v: %v", arguments, err)
		}
	}
	if len(runner.calls) != 4 || runner.calls[0].name != "go" || !slices.Equal(runner.calls[0].arguments, []string{"run", "./cmd/server"}) {
		t.Fatalf("unexpected runner calls: %#v", runner.calls)
	}
	if !slices.Equal(runner.calls[1].arguments, []string{"run", "./cmd/app", "routes"}) || !slices.Equal(runner.calls[2].arguments, []string{"run", "./cmd/app", "jobs"}) || !slices.Equal(runner.calls[3].arguments, []string{"run", "./cmd/app", "seed"}) {
		t.Fatalf("unexpected control calls: %#v", runner.calls)
	}
}

func TestNewAndGenerateResource(t *testing.T) {
	directory := t.TempDir()
	runner := &fakeRunner{}
	var output bytes.Buffer
	command := New(Settings{Directory: directory, Stdout: &output, Stderr: &output, Runner: runner})
	command.SetArgs([]string{"new", "customer-api", "--module", "example.com/customer", "--soro-replace", repositoryRoot(t)})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || !slices.Equal(runner.calls[0].arguments, []string{"mod", "tidy"}) {
		t.Fatalf("new did not resolve dependencies: %#v", runner.calls)
	}
	application := filepath.Join(directory, "customer-api")
	output.Reset()
	command = New(Settings{Directory: application, Stdout: &output, Stderr: &output, Runner: runner})
	command.SetArgs([]string{"generate", "resource", "User", "email:string:unique:index", "active:bool:default=true"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join("app", "models", "user.go"),
		filepath.Join("app", "api", "v1", "user_resource.go"),
	} {
		if _, err := os.Stat(filepath.Join(application, path)); err != nil {
			t.Fatalf("missing generated %s: %v", path, err)
		}
	}
}

func TestOpenAPIGenerateIsConflictSafe(t *testing.T) {
	directory := t.TempDir()
	runner := &fakeRunner{output: "{\"openapi\":\"3.1.0\"}\n"}
	var output bytes.Buffer
	command := New(Settings{Directory: directory, Stdout: &output, Stderr: &output, Runner: runner})
	command.SetArgs([]string{"openapi", "generate"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(directory, "openapi.json"))
	if err != nil || string(contents) != runner.output {
		t.Fatalf("unexpected OpenAPI output %q, %v", contents, err)
	}
	command = New(Settings{Directory: directory, Stdout: &output, Stderr: &output, Runner: runner})
	command.SetArgs([]string{"openapi", "generate"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
}

func TestDatabaseCreateAndDropConfirmation(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "config", "application.yaml"), []byte("database:\n  url: postgres://localhost/customer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var created, dropped string
	settings := Settings{
		Directory: directory, Stdout: io.Discard, Stderr: io.Discard,
		CreateDatabase: func(_ context.Context, databaseURL string) error { created = databaseURL; return nil },
		DropDatabase:   func(_ context.Context, databaseURL string) error { dropped = databaseURL; return nil },
	}
	command := New(settings)
	command.SetArgs([]string{"db", "create"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	command = New(settings)
	command.SetArgs([]string{"db", "drop"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "destructive") {
		t.Fatalf("expected drop confirmation error, got %v", err)
	}
	command = New(settings)
	command.SetArgs([]string{"db", "drop", "--force"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if created != "postgres://localhost/customer" || dropped != created {
		t.Fatalf("unexpected database targets: create=%q drop=%q", created, dropped)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	directory, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
