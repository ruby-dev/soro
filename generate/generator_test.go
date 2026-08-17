package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGeneratedApplicationCompiles(t *testing.T) {
	root := repositoryRoot(t)
	parent := t.TempDir()
	written, err := NewApplication(NewOptions{
		ParentDirectory: parent,
		Name:            "customer-api",
		Module:          "example.com/customer-api",
		SoroReplace:     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) < 10 {
		t.Fatalf("expected application scaffold, got %v", written)
	}
	applicationRoot := filepath.Join(parent, "customer-api")
	generator, err := Open(applicationRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	generator.Now = func() time.Time { return time.Date(2026, 8, 16, 12, 30, 0, 0, time.UTC) }
	if _, err := generator.GenerateResource("User", []string{
		"email:string:unique:index", "first_name:string", "last_name:string", "active:bool:default=true", "account_id:uuid:null",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := generator.GenerateResource("Account", []string{"slug:string:unique", "enabled:bool:default=true"}); err != nil {
		t.Fatal(err)
	}
	if _, err := generator.GenerateJob("SendWelcomeEmail"); err != nil {
		t.Fatal(err)
	}
	if _, err := generator.GenerateMailer("Welcome"); err != nil {
		t.Fatal(err)
	}

	migrationPath := filepath.Join(applicationRoot, "db", "migrations", "20260816123000_create_users.sql")
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migration), "WHERE deleted_at IS NULL") || strings.Contains(string(migration), "users_email_idx") {
		t.Fatalf("generated unique index has incorrect soft-delete semantics:\n%s", migration)
	}

	command := exec.Command(goCommand(t), "mod", "tidy")
	command.Dir = applicationRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated application dependencies do not resolve: %v\n%s", err, output)
	}
	command = exec.Command(goCommand(t), "test", "./...")
	command.Dir = applicationRoot
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err = command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated application does not compile: %v\n%s", err, output)
	}
}

func TestGeneratorRefusesOverwrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	generator, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	generator.Now = func() time.Time { return time.Unix(0, 0).UTC() }
	if _, err := generator.GenerateModel("User", []string{"email:string"}); err != nil {
		t.Fatal(err)
	}
	if _, err := generator.GenerateModel("User", []string{"email:string"}); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
}

func TestGeneratorRefusesUnmanagedRegistry(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	routesPath := filepath.Join(root, "app", "api", "v1", "routes.go")
	if err := os.MkdirAll(filepath.Dir(routesPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(routesPath, []byte("package v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	generator, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := generator.GenerateResource("User", []string{"email:string"}); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("expected unmanaged registry refusal, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "models", "user.go")); !os.IsNotExist(err) {
		t.Fatal("resource generator wrote partial output before registry conflict")
	}
}

func TestNewApplicationRejectsUnsafeTargets(t *testing.T) {
	for _, name := range []string{"", "../bad", "Upper", "two words", "/absolute"} {
		if _, err := NewApplication(NewOptions{ParentDirectory: t.TempDir(), Name: name}); err == nil {
			t.Fatalf("expected application name %q to fail", name)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve repository root")
	}
	return filepath.Dir(filepath.Dir(filename))
}

func goCommand(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go executable unavailable")
	}
	return path
}
