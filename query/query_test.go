package query_test

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	soroerrors "github.com/datasoro/soro/errors"
	"github.com/datasoro/soro/query"
	"github.com/google/uuid"
)

func TestParsePaginationFiltersSearchAndSort(t *testing.T) {
	id := uuid.New()
	definition := testDefinition()
	values := url.Values{
		"page":                    {"2"},
		"per_page":                {"10"},
		"search":                  {"dustin"},
		"sort":                    {"-created_at,name"},
		"filter[active]":          {"true"},
		"filter[id][in]":          {id.String()},
		"filter[created_at][gte]": {"2026-01-01T00:00:00Z"},
	}
	params, err := query.Parse(values, definition)
	if err != nil {
		t.Fatal(err)
	}
	if params.Page != 2 || params.PerPage != 10 || params.Search != "dustin" {
		t.Fatalf("unexpected params: %+v", params)
	}
	if len(params.Filters) != 3 || len(params.Sort) != 2 || !params.Sort[0].Descending {
		t.Fatalf("unexpected filters/sort: %+v", params)
	}
	if value, ok := params.Filters[0].Value.(bool); !ok || !value {
		t.Fatalf("bool filter = %#v", params.Filters[0].Value)
	}
}

func TestParseRejectsUnknownAndInvalidInput(t *testing.T) {
	tests := []url.Values{
		{"unknown": {"x"}},
		{"filter[password]": {"x"}},
		{"filter[active][contains]": {"true"}},
		{"filter[active]": {"1"}},
		{"sort": {"password"}},
		{"page": {"0"}},
		{"per_page": {"101"}},
		{"search": {"one", "two"}},
	}
	for _, values := range tests {
		if _, err := query.Parse(values, testDefinition()); !soroerrors.IsCode(err, soroerrors.CodeInvalidQuery) {
			t.Fatalf("values %v: expected invalid query, got %v", values, err)
		}
	}
}

func TestDefinitionRejectsUnsafeColumns(t *testing.T) {
	definition := query.Definition{Filters: []query.Field{{
		Name: "email", Column: "email; DROP TABLE users", Type: query.String,
	}}}
	if err := definition.Validate(); err == nil {
		t.Fatal("expected unsafe column error")
	}
}

func TestParseAllScalarTypesAndOperators(t *testing.T) {
	id := uuid.New()
	definition := query.Definition{Filters: []query.Field{
		{Name: "string", Column: "string", Type: query.String, Operators: []query.Operator{query.Contains}},
		{Name: "int", Column: "int", Type: query.Int, Operators: []query.Operator{query.Gt}},
		{Name: "float", Column: "float", Type: query.Float, Operators: []query.Operator{query.Lte}},
		{Name: "uuid", Column: "uuid", Type: query.UUID, Operators: []query.Operator{query.Eq}},
		{Name: "date", Column: "date", Type: query.Date, Operators: []query.Operator{query.Lt}},
	}}
	params, err := query.Parse(url.Values{
		"filter[string][contains]": {"abc"},
		"filter[int][gt]":          {"2"},
		"filter[float][lte]":       {"2.5"},
		"filter[uuid]":             {id.String()},
		"filter[date][lt]":         {"2026-08-16"},
	}, definition)
	if err != nil {
		t.Fatal(err)
	}
	if len(params.Filters) != 5 {
		t.Fatalf("filters = %+v", params.Filters)
	}
	var foundDate bool
	for _, filter := range params.Filters {
		if filter.Field.Name == "date" {
			_, foundDate = filter.Value.(time.Time)
		}
	}
	if !foundDate {
		t.Fatal("date was not parsed as time.Time")
	}
}

func TestDefaultSortAndLikeEscaping(t *testing.T) {
	params, err := query.Parse(url.Values{}, testDefinition())
	if err != nil {
		t.Fatal(err)
	}
	want := []query.Sort{{Field: "created_at", Column: "created_at", Descending: true}}
	if !reflect.DeepEqual(params.Sort, want) {
		t.Fatalf("sort = %+v, want %+v", params.Sort, want)
	}
	if got := query.EscapeLike("50%!_done"); got != "50!%!!!_done" {
		t.Fatalf("EscapeLike = %q", got)
	}
}

func testDefinition() query.Definition {
	return query.Definition{
		Filters: []query.Field{
			{Name: "active", Column: "active", Type: query.Bool, Operators: []query.Operator{query.Eq, query.Neq}},
			{Name: "id", Column: "id", Type: query.UUID, Operators: []query.Operator{query.Eq, query.In}},
			{Name: "created_at", Column: "created_at", Type: query.Timestamp, Operators: []query.Operator{query.Eq, query.Gte}},
		},
		Searchable: []string{"name", "email"},
		Sortable: []query.SortField{
			{Name: "created_at", Column: "created_at"},
			{Name: "name", Column: "name"},
		},
		DefaultSort: []query.Sort{{Field: "created_at", Descending: true}},
	}
}
