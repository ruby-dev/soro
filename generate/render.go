package generate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func renderModel(name Name, fields []Field) ([]byte, error) {
	var source bytes.Buffer
	writeLine(&source, "package models")
	writeLine(&source, "")
	writeLine(&source, "import (")
	writeLine(&source, "\t%q", FrameworkModule+"/model")
	if hasType(fields, UUID) {
		writeLine(&source, "\t%q", "github.com/google/uuid")
	}
	if hasType(fields, Time) {
		writeLine(&source, "\t%q", "time")
	}
	writeLine(&source, ")")
	writeLine(&source, "")
	writeLine(&source, "type %s struct {", name.Singular)
	writeLine(&source, "\tmodel.Base")
	for _, field := range fields {
		bunOptions := []string{field.Name}
		if field.Nullable {
			bunOptions = append(bunOptions, "nullzero")
		} else {
			bunOptions = append(bunOptions, "notnull")
		}
		validation := validationTag(field)
		tag := fmt.Sprintf("bun:\"%s\" json:\"%s,omitempty\"", strings.Join(bunOptions, ","), field.Name)
		if validation != "" {
			tag += " validate:\"" + validation + "\""
		}
		writeLine(&source, "\t%s %s `%s`", field.GoName, field.GoType(), tag)
	}
	writeLine(&source, "}")
	return formatted(source.String())
}

func renderModelTest(name Name) ([]byte, error) {
	return formatted(fmt.Sprintf(`package models

import (
	"testing"

	"github.com/datasoro/soro/model"
)

func Test%sEmbedsSoroBase(t *testing.T) {
	var entity model.Entity = &%s{}
	if entity.SoroBase() == nil {
		t.Fatal("Soro base is unavailable")
	}
}
`, name.Singular, name.Singular))
}

func renderSerializer(module string, name Name, fields []Field) ([]byte, error) {
	var source bytes.Buffer
	writeLine(&source, "package serializers")
	writeLine(&source, "")
	writeLine(&source, "import (")
	writeLine(&source, "\t%q", "context")
	writeLine(&source, "\t%q", "time")
	writeLine(&source, "\t%q", module+"/app/models")
	writeLine(&source, "\t%q", FrameworkModule+"/model")
	writeLine(&source, "\t%q", "github.com/google/uuid")
	writeLine(&source, ")")
	writeLine(&source, "")
	writeLine(&source, "type %sResponse struct {", name.Singular)
	writeLine(&source, "\tID uuid.UUID `json:\"id\"`")
	writeLine(&source, "\tName string `json:\"name\"`")
	writeLine(&source, "\tDescription string `json:\"description\"`")
	writeLine(&source, "\tMetadata model.Metadata `json:\"metadata\"`")
	for _, field := range fields {
		writeLine(&source, "\t%s %s `json:\"%s,omitempty\"`", field.GoName, field.GoType(), field.Name)
	}
	writeLine(&source, "\tCreatedAt time.Time `json:\"created_at\"`")
	writeLine(&source, "\tUpdatedAt time.Time `json:\"updated_at\"`")
	writeLine(&source, "}")
	writeLine(&source, "")
	writeLine(&source, "type %sSerializer struct{}", name.Singular)
	writeLine(&source, "")
	writeLine(&source, "func (%sSerializer) Serialize(_ context.Context, entity *models.%s) (%sResponse, error) {", name.Singular, name.Singular, name.Singular)
	writeLine(&source, "\treturn %sResponse{", name.Singular)
	writeLine(&source, "\t\tID: entity.ID, Name: entity.Name, Description: entity.Description, Metadata: entity.Metadata,")
	for _, field := range fields {
		writeLine(&source, "\t\t%s: entity.%s,", field.GoName, field.GoName)
	}
	writeLine(&source, "\t\tCreatedAt: entity.CreatedAt, UpdatedAt: entity.UpdatedAt,")
	writeLine(&source, "\t}, nil")
	writeLine(&source, "}")
	return formatted(source.String())
}

func renderValidator(name Name, fields []Field) ([]byte, error) {
	var source bytes.Buffer
	writeLine(&source, "package validators")
	writeLine(&source, "")
	writeLine(&source, "import (")
	writeLine(&source, "\t%q", FrameworkModule+"/model")
	if hasType(fields, UUID) {
		writeLine(&source, "\t%q", "github.com/google/uuid")
	}
	if hasType(fields, Time) {
		writeLine(&source, "\t%q", "time")
	}
	writeLine(&source, ")")
	writeLine(&source, "")
	writeLine(&source, "type Create%sInput struct {", name.Singular)
	writeLine(&source, "\tName string `json:\"name\" validate:\"required,max=255\"`")
	writeLine(&source, "\tDescription string `json:\"description,omitempty\"`")
	writeLine(&source, "\tMetadata model.Metadata `json:\"metadata,omitempty\"`")
	for _, field := range fields {
		tag := validationTag(field)
		if tag != "" {
			tag = " validate:\"" + tag + "\""
		}
		writeLine(&source, "\t%s %s `json:\"%s,omitempty\"%s`", field.GoName, field.CreateGoType(), field.Name, tag)
	}
	writeLine(&source, "}")
	writeLine(&source, "")
	writeLine(&source, "type Update%sInput struct {", name.Singular)
	writeLine(&source, "\tName *string `json:\"name,omitempty\" validate:\"omitempty,min=1,max=255\"`")
	writeLine(&source, "\tDescription *string `json:\"description,omitempty\"`")
	writeLine(&source, "\tMetadata *model.Metadata `json:\"metadata,omitempty\"`")
	for _, field := range fields {
		writeLine(&source, "\t%s *%s `json:\"%s,omitempty\"`", field.GoName, field.GoType(), field.Name)
	}
	writeLine(&source, "}")
	return formatted(source.String())
}

func renderResource(module string, name Name, fields []Field) ([]byte, error) {
	var source bytes.Buffer
	writeLine(&source, "// soro:resource %s", name.Singular)
	writeLine(&source, "package v1")
	writeLine(&source, "")
	writeLine(&source, "import (")
	writeLine(&source, "\t%q", "context")
	writeLine(&source, "\t%q", module+"/app/models")
	writeLine(&source, "\t%q", module+"/app/serializers")
	writeLine(&source, "\t%q", module+"/app/validators")
	writeLine(&source, "\t%q", FrameworkModule+"/api")
	writeLine(&source, "\t%q", FrameworkModule+"/model")
	writeLine(&source, "\t%q", FrameworkModule+"/query")
	writeLine(&source, "\t%q", FrameworkModule+"/repository")
	if hasTimeDefault(fields) {
		writeLine(&source, "\t%q", "time")
	}
	writeLine(&source, ")")
	writeLine(&source, "")
	writeLine(&source, "func New%sResource(dbRepository *repository.Repository[models.%s]) (*api.Resource[models.%s, validators.Create%sInput, validators.Update%sInput, serializers.%sResponse], error) {", name.Singular, name.Singular, name.Singular, name.Singular, name.Singular, name.Singular)
	writeLine(&source, "\treturn api.NewResource(api.ResourceConfig[models.%s, validators.Create%sInput, validators.Update%sInput, serializers.%sResponse]{", name.Singular, name.Singular, name.Singular, name.Singular)
	writeLine(&source, "\t\tName: %q,", name.Plural)
	writeLine(&source, "\t\tRepository: dbRepository,")
	writeLine(&source, "\t\tSerializer: serializers.%sSerializer{},", name.Singular)
	writeLine(&source, "\t\tCreateEntity: func(_ context.Context, input validators.Create%sInput) (*models.%s, error) {", name.Singular, name.Singular)
	writeLine(&source, "\t\t\tentity := &models.%s{Base: model.Base{Name: input.Name, Description: input.Description, Metadata: input.Metadata},", name.Singular)
	for _, field := range fields {
		if !field.HasDefault {
			writeLine(&source, "\t\t\t\t%s: input.%s,", field.GoName, field.GoName)
		}
	}
	writeLine(&source, "\t\t\t}")
	for _, field := range fields {
		if field.HasDefault {
			writeLine(&source, "\t\t\tentity.%s = %s", field.GoName, field.GoDefault())
			writeLine(&source, "\t\t\tif input.%s != nil { entity.%s = *input.%s }", field.GoName, field.GoName, field.GoName)
		}
	}
	writeLine(&source, "\t\t\treturn entity, nil")
	writeLine(&source, "\t\t},")
	writeLine(&source, "\t\tUpdateEntity: func(_ context.Context, entity *models.%s, input validators.Update%sInput) error {", name.Singular, name.Singular)
	writeLine(&source, "\t\t\tif input.Name != nil { entity.Name = *input.Name }")
	writeLine(&source, "\t\t\tif input.Description != nil { entity.Description = *input.Description }")
	writeLine(&source, "\t\t\tif input.Metadata != nil { entity.Metadata = *input.Metadata }")
	for _, field := range fields {
		writeLine(&source, "\t\t\tif input.%s != nil { entity.%s = *input.%s }", field.GoName, field.GoName, field.GoName)
	}
	writeLine(&source, "\t\t\treturn nil")
	writeLine(&source, "\t\t},")
	writeLine(&source, "\t\tQuery: query.Definition{")
	writeLine(&source, "\t\t\tFilters: []query.Field{")
	for _, field := range fields {
		if queryType(field.Type) == "" {
			continue
		}
		writeLine(&source, "\t\t\t\t{Name: %q, Column: %q, Type: query.%s, Operators: []query.Operator{%s}},", field.Name, field.Name, queryType(field.Type), queryOperators(field.Type))
	}
	writeLine(&source, "\t\t\t},")
	writeLine(&source, "\t\t\tSearchable: []string{%s},", quoted(searchable(fields)))
	writeLine(&source, "\t\t\tSortable: []query.SortField{")
	writeLine(&source, "\t\t\t\t{Name: \"name\", Column: \"name\"}, {Name: \"created_at\", Column: \"created_at\"},")
	for _, field := range fields {
		if field.Type != JSON {
			writeLine(&source, "\t\t\t\t{Name: %q, Column: %q},", field.Name, field.Name)
		}
	}
	writeLine(&source, "\t\t\t},")
	writeLine(&source, "\t\t\tDefaultSort: []query.Sort{{Field: \"created_at\", Descending: true}},")
	writeLine(&source, "\t\t},")
	writeLine(&source, "\t})")
	writeLine(&source, "}")
	return formatted(source.String())
}

func renderResourceTest(name Name) ([]byte, error) {
	return formatted(fmt.Sprintf(`package v1

import "testing"

func TestNew%sResourceRequiresRepository(t *testing.T) {
	if _, err := New%sResource(nil); err == nil {
		t.Fatal("expected a repository configuration error")
	}
}
`, name.Singular, name.Singular))
}

func renderRoutes(module string, resources []Name) ([]byte, error) {
	var source bytes.Buffer
	writeLine(&source, "// Code generated by Soro. DO NOT EDIT.")
	writeLine(&source, "package v1")
	writeLine(&source, "")
	writeLine(&source, "import (")
	writeLine(&source, "\t%q", FrameworkModule)
	writeLine(&source, "\t%q", FrameworkModule+"/api")
	if len(resources) > 0 {
		writeLine(&source, "\t%q", module+"/app/models")
		writeLine(&source, "\t%q", FrameworkModule+"/repository")
	}
	writeLine(&source, ")")
	writeLine(&source, "")
	writeLine(&source, "func Register(app *soro.App) error {")
	writeLine(&source, "\tvar registrationErr error")
	writeLine(&source, "\tif err := app.API.Version(\"v1\", func(router *api.Router) {")
	for _, resource := range resources {
		writeLine(&source, "\t\t{")
		writeLine(&source, "\t\t\tif registrationErr != nil { return }")
		writeLine(&source, "\t\t\tconfigured, err := New%sResource(repository.New[models.%s](app.DB))", resource.Singular, resource.Singular)
		writeLine(&source, "\t\t\tif err != nil { registrationErr = err; return }")
		writeLine(&source, "\t\t\tregistrationErr = router.Resource(%q, configured)", "/"+resource.Table)
		writeLine(&source, "\t\t}")
	}
	writeLine(&source, "\t}); err != nil { return err }")
	writeLine(&source, "\treturn registrationErr")
	writeLine(&source, "}")
	return formatted(source.String())
}

func renderMigration(name Name, fields []Field) ([]byte, error) {
	var source bytes.Buffer
	writeLine(&source, "-- +soro Up")
	writeLine(&source, "CREATE TABLE %s (", name.Table)
	columns := []string{
		"    id UUID PRIMARY KEY",
		"    name VARCHAR(255) NOT NULL DEFAULT ''",
		"    description TEXT NOT NULL DEFAULT ''",
		"    metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"    deleted_at TIMESTAMPTZ NULL",
		"    created_by UUID NULL",
		"    updated_by UUID NULL",
		"    deleted_by UUID NULL",
	}
	for _, field := range fields {
		column := "    " + field.Name + " " + field.SQLType()
		if !field.Nullable {
			column += " NOT NULL"
		}
		if field.HasDefault {
			value, err := field.SQLDefault()
			if err != nil {
				return nil, err
			}
			column += " DEFAULT " + value
		}
		columns = append(columns, column)
	}
	writeLine(&source, "%s", strings.Join(columns, ",\n"))
	writeLine(&source, ");")
	for _, field := range fields {
		if field.Unique {
			writeLine(&source, "CREATE UNIQUE INDEX %s_%s_unique ON %s (%s) WHERE deleted_at IS NULL;", name.Table, field.Name, name.Table, field.Name)
		} else if field.Index {
			writeLine(&source, "CREATE INDEX %s_%s_idx ON %s (%s);", name.Table, field.Name, name.Table, field.Name)
		}
	}
	writeLine(&source, "")
	writeLine(&source, "-- +soro Down")
	writeLine(&source, "DROP TABLE %s;", name.Table)
	return source.Bytes(), nil
}

func (generator *Generator) resourceRegistrations(current Name) ([]Name, error) {
	directory := filepath.Join(generator.Root, "app", "api", "v1")
	entries, err := os.ReadDir(directory)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	byName := map[string]Name{current.Singular: current}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_resource.go") {
			continue
		}
		contents, readErr := os.ReadFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		for _, line := range strings.Split(string(contents), "\n") {
			const marker = "// soro:resource "
			if strings.HasPrefix(line, marker) {
				parsed, parseErr := ParseName(strings.TrimPrefix(line, marker))
				if parseErr != nil {
					return nil, parseErr
				}
				byName[parsed.Singular] = parsed
				break
			}
		}
	}
	result := make([]Name, 0, len(byName))
	for _, parsed := range byName {
		result = append(result, parsed)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Singular < result[j].Singular })
	return result, nil
}

func hasType(fields []Field, fieldType FieldType) bool {
	for _, field := range fields {
		if field.Type == fieldType {
			return true
		}
	}
	return false
}

func hasTimeDefault(fields []Field) bool {
	for _, field := range fields {
		if field.Type == Time && field.HasDefault {
			return true
		}
	}
	return false
}

func validationTag(field Field) string {
	var tags []string
	if !field.Nullable && !field.HasDefault && (field.Type == String || field.Type == Text || field.Type == UUID) {
		tags = append(tags, "required")
	}
	if field.Type == String {
		tags = append(tags, "max=255")
	}
	if field.Name == "email" || strings.HasSuffix(field.Name, "_email") {
		tags = append(tags, "email")
	}
	return strings.Join(tags, ",")
}

func queryType(fieldType FieldType) string {
	return map[FieldType]string{String: "String", Text: "String", Bool: "Bool", UUID: "UUID", Int: "Int", Float: "Float", Time: "Timestamp"}[fieldType]
}

func queryOperators(fieldType FieldType) string {
	switch fieldType {
	case String, Text:
		return "query.Eq, query.Neq, query.Contains"
	case Bool, UUID:
		return "query.Eq, query.Neq"
	default:
		return "query.Eq, query.Neq, query.Gt, query.Gte, query.Lt, query.Lte"
	}
}

func searchable(fields []Field) []string {
	values := []string{"name", "description"}
	for _, field := range fields {
		if field.Type == String || field.Type == Text {
			values = append(values, field.Name)
		}
	}
	return values
}

func quoted(values []string) string {
	quotedValues := make([]string, len(values))
	for index, value := range values {
		quotedValues[index] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quotedValues, ", ")
}
