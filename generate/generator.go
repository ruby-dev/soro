package generate

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const FrameworkModule = "github.com/ruby-dev/soro"

type Generator struct {
	Root         string
	Module       string
	Force        bool
	Now          func() time.Time
	Version      string
	transformers []Transformer
}

// Transformer customizes rendered file content without controlling its path or
// write behavior. Transformers run in registration order before final Go
// formatting and preflight checks.
type Transformer func(path string, content []byte) ([]byte, error)

func (generator *Generator) UseTransformers(transformers ...Transformer) error {
	if generator == nil {
		return fmt.Errorf("generator is required")
	}
	for _, transformer := range transformers {
		if transformer == nil {
			return fmt.Errorf("generator transformer cannot be nil")
		}
	}
	generator.transformers = append(generator.transformers, transformers...)
	return nil
}

type generatedFile struct {
	path    string
	content []byte
	goFile  bool
	managed bool
}

func Open(root string, force bool) (*Generator, error) {
	module, err := ReadModule(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	return &Generator{Root: root, Module: module, Force: force, Now: time.Now, Version: "v0.0.0"}, nil
}

func ReadModule(path string) (string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Go module: %w", err)
	}
	for _, line := range strings.Split(string(contents), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "module" {
			if err := ValidateModule(fields[1]); err != nil {
				return "", err
			}
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("go.mod does not declare a module")
}

func (generator *Generator) GenerateModel(rawName string, specifications []string) ([]string, error) {
	name, fields, err := parseComponent(rawName, specifications)
	if err != nil {
		return nil, err
	}
	modelSource, err := renderModel(name, fields)
	if err != nil {
		return nil, err
	}
	migrationSource, err := renderMigration(name, fields)
	if err != nil {
		return nil, err
	}
	modelTest, err := renderModelTest(name)
	if err != nil {
		return nil, err
	}
	factorySource, err := renderFactory(generator.Module, name, fields)
	if err != nil {
		return nil, err
	}
	stamp := generator.Now().UTC().Format("20060102150405")
	return generator.writeFiles([]generatedFile{
		{path: filepath.Join("app", "models", name.Snake+".go"), content: modelSource, goFile: true},
		{path: filepath.Join("app", "models", name.Snake+"_test.go"), content: modelTest, goFile: true},
		{path: filepath.Join("app", "factories", name.Snake+".go"), content: factorySource, goFile: true},
		{path: filepath.Join("db", "migrations", stamp+"_create_"+name.Table+".sql"), content: migrationSource},
	})
}

func (generator *Generator) GenerateMigration(rawName string) ([]string, error) {
	name, err := migrationName(rawName)
	if err != nil {
		return nil, err
	}
	stamp := generator.Now().UTC().Format("20060102150405")
	contents := []byte("-- +soro Up\n-- Write forward PostgreSQL SQL here.\nSELECT 1;\n\n-- +soro Down\n-- Write rollback PostgreSQL SQL here.\nSELECT 1;\n")
	return generator.writeFiles([]generatedFile{{path: filepath.Join("db", "migrations", stamp+"_"+name+".sql"), content: contents}})
}

func (generator *Generator) GenerateSerializer(rawName string, specifications []string) ([]string, error) {
	name, fields, err := parseComponent(rawName, specifications)
	if err != nil {
		return nil, err
	}
	source, err := renderSerializer(generator.Module, name, fields)
	if err != nil {
		return nil, err
	}
	return generator.writeFiles([]generatedFile{{path: filepath.Join("app", "serializers", name.Snake+".go"), content: source, goFile: true}})
}

func (generator *Generator) GenerateValidator(rawName string, specifications []string) ([]string, error) {
	name, fields, err := parseComponent(rawName, specifications)
	if err != nil {
		return nil, err
	}
	source, err := renderValidator(name, fields)
	if err != nil {
		return nil, err
	}
	return generator.writeFiles([]generatedFile{{path: filepath.Join("app", "validators", name.Snake+".go"), content: source, goFile: true}})
}

func (generator *Generator) GenerateJob(rawName string) ([]string, error) {
	name, err := ParseName(rawName)
	if err != nil {
		return nil, err
	}
	source, err := formatted(fmt.Sprintf(`// soro:job %[1]s
package jobs

import "context"

type %[1]s struct{}

func (%[1]s) Kind() string { return %q }

func Perform%[1]s(ctx context.Context, args %[1]s) error {
	_ = ctx
	_ = args
	return nil
}
`, name.Singular, name.Snake))
	if err != nil {
		return nil, err
	}
	registrations, err := generator.jobRegistrations(name)
	if err != nil {
		return nil, err
	}
	registry, err := renderJobRegistry(registrations)
	if err != nil {
		return nil, err
	}
	return generator.writeFiles([]generatedFile{
		{path: filepath.Join("app", "jobs", name.Snake+".go"), content: source, goFile: true},
		{path: filepath.Join("app", "jobs", "register.go"), content: registry, goFile: true, managed: true},
	})
}

func (generator *Generator) GenerateMailer(rawName string) ([]string, error) {
	name, err := ParseName(rawName)
	if err != nil {
		return nil, err
	}
	source, err := formatted(fmt.Sprintf(`package mailers

import (
	"fmt"

	"github.com/ruby-dev/soro/mail"
)

type %[1]sData struct {
	Recipient string
}

func %[1]s(client *mail.Client, data %[1]sData) (*mail.Delivery, error) {
	templates, err := mail.ParseTemplates(%q, %q, %q, %q)
	if err != nil {
		return nil, err
	}
	subject, textBody, htmlBody, err := templates.Render(data)
	if err != nil {
		return nil, err
	}
	if data.Recipient == "" {
		return nil, fmt.Errorf("recipient is required")
	}
	return client.Delivery(&mail.Message{To: []string{data.Recipient}, Subject: subject, Text: textBody, HTML: htmlBody}), nil
}
`, name.Singular, name.Snake, name.Singular, name.Singular+"\n", "<p>"+name.Singular+"</p>"))
	if err != nil {
		return nil, err
	}
	return generator.writeFiles([]generatedFile{{path: filepath.Join("app", "mailers", name.Snake+".go"), content: source, goFile: true}})
}

func (generator *Generator) GenerateResource(rawName string, specifications []string) ([]string, error) {
	name, fields, err := parseComponent(rawName, specifications)
	if err != nil {
		return nil, err
	}
	renderers := []struct {
		path   string
		render func() ([]byte, error)
	}{
		{filepath.Join("app", "models", name.Snake+".go"), func() ([]byte, error) { return renderModel(name, fields) }},
		{filepath.Join("app", "models", name.Snake+"_test.go"), func() ([]byte, error) { return renderModelTest(name) }},
		{filepath.Join("app", "factories", name.Snake+".go"), func() ([]byte, error) { return renderFactory(generator.Module, name, fields) }},
		{filepath.Join("app", "serializers", name.Snake+".go"), func() ([]byte, error) { return renderSerializer(generator.Module, name, fields) }},
		{filepath.Join("app", "validators", name.Snake+".go"), func() ([]byte, error) { return renderValidator(name, fields) }},
		{filepath.Join("app", "api", "v1", name.Snake+"_resource.go"), func() ([]byte, error) { return renderResource(generator.Module, name, fields) }},
		{filepath.Join("app", "api", "v1", name.Snake+"_resource_test.go"), func() ([]byte, error) { return renderResourceTest(name) }},
	}
	files := make([]generatedFile, 0, len(renderers)+2)
	for _, configured := range renderers {
		contents, renderErr := configured.render()
		if renderErr != nil {
			return nil, renderErr
		}
		files = append(files, generatedFile{path: configured.path, content: contents, goFile: true})
	}
	migrationSource, err := renderMigration(name, fields)
	if err != nil {
		return nil, err
	}
	stamp := generator.Now().UTC().Format("20060102150405")
	files = append(files, generatedFile{path: filepath.Join("db", "migrations", stamp+"_create_"+name.Table+".sql"), content: migrationSource})
	registrations, err := generator.resourceRegistrations(name)
	if err != nil {
		return nil, err
	}
	routes, err := renderRoutes(generator.Module, registrations)
	if err != nil {
		return nil, err
	}
	files = append(files, generatedFile{path: filepath.Join("app", "api", "v1", "routes.go"), content: routes, goFile: true, managed: true})
	return generator.writeFiles(files)
}

func parseComponent(rawName string, specifications []string) (Name, []Field, error) {
	name, err := ParseName(rawName)
	if err != nil {
		return Name{}, nil, err
	}
	fields, err := ParseFields(specifications)
	if err != nil {
		return Name{}, nil, err
	}
	return name, fields, nil
}

func migrationName(raw string) (string, error) {
	words := wordsOf(raw)
	if len(words) == 0 {
		return "", fmt.Errorf("migration name is required")
	}
	name := strings.Join(words, "_")
	if !fieldNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid migration name %q", raw)
	}
	return name, nil
}

func (generator *Generator) writeFiles(files []generatedFile) ([]string, error) {
	seen := make(map[string]struct{}, len(files))
	for index := range files {
		for transformerIndex, transformer := range generator.transformers {
			input := append([]byte(nil), files[index].content...)
			transformed, err := transformer(filepath.ToSlash(files[index].path), input)
			if err != nil {
				return nil, fmt.Errorf("transform generated %s with transformer %d: %w", files[index].path, transformerIndex+1, err)
			}
			if transformed == nil {
				return nil, fmt.Errorf("transform generated %s with transformer %d: content cannot be nil", files[index].path, transformerIndex+1)
			}
			files[index].content = append([]byte(nil), transformed...)
		}
		if files[index].managed && !bytes.HasPrefix(files[index].content, []byte("// Code generated by Soro. DO NOT EDIT.")) {
			return nil, fmt.Errorf("transform generated %s: managed marker must be preserved", files[index].path)
		}
		if files[index].goFile {
			formattedSource, err := format.Source(files[index].content)
			if err != nil {
				return nil, fmt.Errorf("format generated %s: %w\n%s", files[index].path, err, files[index].content)
			}
			files[index].content = formattedSource
		}
		absolute := filepath.Join(generator.Root, files[index].path)
		if _, exists := seen[absolute]; exists {
			return nil, fmt.Errorf("generator produced duplicate path %s", files[index].path)
		}
		seen[absolute] = struct{}{}
		if _, err := os.Stat(absolute); err == nil && !generator.Force && !files[index].managed {
			return nil, fmt.Errorf("refusing to overwrite %s (use --force)", files[index].path)
		} else if err == nil && !generator.Force && files[index].managed {
			existing, readErr := os.ReadFile(absolute)
			if readErr != nil {
				return nil, readErr
			}
			if !bytes.HasPrefix(existing, []byte("// Code generated by Soro. DO NOT EDIT.")) {
				return nil, fmt.Errorf("refusing to overwrite unmanaged %s (use --force)", files[index].path)
			}
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	written := make([]string, 0, len(files))
	for _, file := range files {
		absolute := filepath.Join(generator.Root, file.path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
			return written, fmt.Errorf("create directory for %s: %w", file.path, err)
		}
		temporary, err := os.CreateTemp(filepath.Dir(absolute), ".soro-generate-*")
		if err != nil {
			return written, err
		}
		temporaryName := temporary.Name()
		if _, err = temporary.Write(file.content); err == nil {
			err = temporary.Chmod(0o644)
		}
		closeErr := temporary.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(temporaryName, absolute)
		}
		if err != nil {
			_ = os.Remove(temporaryName)
			return written, fmt.Errorf("write %s: %w", file.path, err)
		}
		written = append(written, file.path)
	}
	return written, nil
}

func formatted(source string) ([]byte, error) {
	result, err := format.Source([]byte(source))
	if err != nil {
		return nil, fmt.Errorf("format generated Go: %w\n%s", err, source)
	}
	return result, nil
}

func writeLine(buffer *bytes.Buffer, format string, arguments ...any) {
	fmt.Fprintf(buffer, format+"\n", arguments...)
}
