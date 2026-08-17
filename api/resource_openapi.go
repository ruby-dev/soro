package api

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"
	"github.com/datasoro/soro/query"
)

func queryParameters(definition query.Definition) []*huma.Param {
	definition = definition.WithDefaults()
	minimum := float64(1)
	maximum := float64(definition.MaxPerPage)
	parameters := []*huma.Param{
		{Name: "page", In: "query", Description: "Page number", Schema: &huma.Schema{Type: "integer", Minimum: &minimum, Default: definition.DefaultPage}},
		{Name: "per_page", In: "query", Description: "Records per page", Schema: &huma.Schema{Type: "integer", Minimum: &minimum, Maximum: &maximum, Default: definition.DefaultPerPage}},
	}
	if len(definition.Searchable) > 0 {
		parameters = append(parameters, &huma.Param{Name: "search", In: "query", Description: "Search permitted text fields", Schema: &huma.Schema{Type: "string"}})
	}
	if len(definition.Sortable) > 0 {
		parameters = append(parameters, &huma.Param{Name: "sort", In: "query", Description: "Comma-separated allowed sort fields; prefix descending fields with -", Schema: &huma.Schema{Type: "string"}})
	}
	for _, field := range definition.Filters {
		for _, operator := range field.Operators {
			name := "filter[" + field.Name + "]"
			if operator != query.Eq {
				name += "[" + string(operator) + "]"
			}
			parameters = append(parameters, &huma.Param{
				Name: name, In: "query", Description: fmt.Sprintf("Filter %s using %s", field.Name, operator),
				Schema: querySchema(field.Type, operator),
			})
		}
	}
	return parameters
}

func querySchema(valueType query.ValueType, operator query.Operator) *huma.Schema {
	if operator == query.In {
		return &huma.Schema{Type: "string", Description: "Comma-separated values"}
	}
	schema := &huma.Schema{Type: "string"}
	switch valueType {
	case query.Bool:
		schema.Type = "boolean"
	case query.Int:
		schema.Type = "integer"
	case query.Float:
		schema.Type = "number"
	case query.UUID:
		schema.Format = "uuid"
	case query.Date:
		schema.Format = "date"
	case query.Timestamp:
		schema.Format = "date-time"
	}
	return schema
}
