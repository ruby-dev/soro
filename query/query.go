// Package query parses and applies safe, resource-defined PostgreSQL queries.
package query

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	soroerrors "github.com/datasoro/soro/errors"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Operator string

const (
	Eq       Operator = "eq"
	Neq      Operator = "neq"
	Gt       Operator = "gt"
	Gte      Operator = "gte"
	Lt       Operator = "lt"
	Lte      Operator = "lte"
	In       Operator = "in"
	Contains Operator = "contains"
)

type ValueType string

const (
	String    ValueType = "string"
	Bool      ValueType = "bool"
	Int       ValueType = "int"
	Float     ValueType = "float"
	UUID      ValueType = "uuid"
	Date      ValueType = "date"
	Timestamp ValueType = "timestamp"
)

type Field struct {
	Name      string
	Column    string
	Type      ValueType
	Operators []Operator
}

type SortField struct {
	Name   string
	Column string
}

type Definition struct {
	Filters        []Field
	Searchable     []string
	Sortable       []SortField
	DefaultSort    []Sort
	DefaultPage    int
	DefaultPerPage int
	MaxPerPage     int
}

type Filter struct {
	Field    Field
	Operator Operator
	Value    any
}

type Sort struct {
	Field      string
	Column     string
	Descending bool
}

type Params struct {
	Page    int
	PerPage int
	Search  string
	Filters []Filter
	Sort    []Sort
}

var (
	apiNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	columnPattern  = regexp.MustCompile(`^[a-z_][a-z0-9_]*(\.[a-z_][a-z0-9_]*)?$`)
	filterPattern  = regexp.MustCompile(`^filter\[([a-z][a-z0-9_]*)\](?:\[([a-z]+)\])?$`)
)

func (definition Definition) WithDefaults() Definition {
	if definition.DefaultPage == 0 {
		definition.DefaultPage = 1
	}
	if definition.DefaultPerPage == 0 {
		definition.DefaultPerPage = 25
	}
	if definition.MaxPerPage == 0 {
		definition.MaxPerPage = 100
	}
	for index := range definition.Filters {
		if len(definition.Filters[index].Operators) == 0 {
			definition.Filters[index].Operators = []Operator{Eq}
		}
	}
	return definition
}

func (definition Definition) Validate() error {
	definition = definition.WithDefaults()
	if definition.DefaultPage < 1 {
		return fmt.Errorf("query default page must be at least 1")
	}
	if definition.DefaultPerPage < 1 || definition.MaxPerPage < 1 || definition.DefaultPerPage > definition.MaxPerPage {
		return fmt.Errorf("query per-page defaults are invalid")
	}
	filterNames := make(map[string]struct{}, len(definition.Filters))
	for _, field := range definition.Filters {
		if !apiNamePattern.MatchString(field.Name) {
			return fmt.Errorf("invalid filter name %q", field.Name)
		}
		if !columnPattern.MatchString(field.Column) {
			return fmt.Errorf("invalid filter column %q", field.Column)
		}
		if !knownType(field.Type) {
			return fmt.Errorf("invalid type %q for filter %s", field.Type, field.Name)
		}
		if _, exists := filterNames[field.Name]; exists {
			return fmt.Errorf("duplicate filter %q", field.Name)
		}
		filterNames[field.Name] = struct{}{}
		operators := make(map[Operator]struct{}, len(field.Operators))
		for _, operator := range field.Operators {
			if !operatorAllowed(field.Type, operator) {
				return fmt.Errorf("operator %q is not valid for %s filter %s", operator, field.Type, field.Name)
			}
			if _, exists := operators[operator]; exists {
				return fmt.Errorf("duplicate operator %q for filter %s", operator, field.Name)
			}
			operators[operator] = struct{}{}
		}
	}
	for _, column := range definition.Searchable {
		if !columnPattern.MatchString(column) {
			return fmt.Errorf("invalid search column %q", column)
		}
	}
	sortNames := make(map[string]string, len(definition.Sortable))
	for _, field := range definition.Sortable {
		if !apiNamePattern.MatchString(field.Name) || !columnPattern.MatchString(field.Column) {
			return fmt.Errorf("invalid sort definition %q -> %q", field.Name, field.Column)
		}
		if _, exists := sortNames[field.Name]; exists {
			return fmt.Errorf("duplicate sort %q", field.Name)
		}
		sortNames[field.Name] = field.Column
	}
	for _, configured := range definition.DefaultSort {
		column, exists := sortNames[configured.Field]
		if !exists {
			return fmt.Errorf("default sort %q is not sortable", configured.Field)
		}
		if configured.Column != "" && configured.Column != column {
			return fmt.Errorf("default sort %q has an inconsistent column", configured.Field)
		}
	}
	return nil
}

func Parse(values url.Values, definition Definition) (Params, error) {
	definition = definition.WithDefaults()
	if err := definition.Validate(); err != nil {
		return Params{}, soroerrors.InvalidQuery(err.Error())
	}
	params := Params{Page: definition.DefaultPage, PerPage: definition.DefaultPerPage}
	var err error
	if raw, present := first(values, "page"); present {
		params.Page, err = positiveInteger("page", raw)
		if err != nil {
			return Params{}, err
		}
	}
	if raw, present := first(values, "per_page"); present {
		params.PerPage, err = positiveInteger("per_page", raw)
		if err != nil {
			return Params{}, err
		}
		if params.PerPage > definition.MaxPerPage {
			return Params{}, soroerrors.InvalidQuery(fmt.Sprintf("per_page must be at most %d", definition.MaxPerPage))
		}
	}
	if rawValues, present := values["search"]; present {
		if len(rawValues) != 1 {
			return Params{}, soroerrors.InvalidQuery("search must be provided exactly once")
		}
		raw := rawValues[0]
		if len(definition.Searchable) == 0 {
			return Params{}, soroerrors.InvalidQuery("search is not supported")
		}
		params.Search = raw
	}
	if raw, present := first(values, "sort"); present {
		params.Sort, err = parseSort(raw, definition.Sortable)
		if err != nil {
			return Params{}, err
		}
	} else {
		params.Sort = resolveDefaultSort(definition.DefaultSort, definition.Sortable)
	}

	filterFields := make(map[string]Field, len(definition.Filters))
	for _, field := range definition.Filters {
		filterFields[field.Name] = field
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "page" || key == "per_page" || key == "search" || key == "sort" {
			continue
		}
		matches := filterPattern.FindStringSubmatch(key)
		if matches == nil {
			return Params{}, soroerrors.InvalidQuery("unsupported query parameter " + key)
		}
		field, ok := filterFields[matches[1]]
		if !ok {
			return Params{}, soroerrors.InvalidQuery("unsupported filter " + matches[1])
		}
		operator := Eq
		if matches[2] != "" {
			operator = Operator(matches[2])
		}
		if !containsOperator(field.Operators, operator) {
			return Params{}, soroerrors.InvalidQuery(fmt.Sprintf("unsupported operator %s for filter %s", operator, field.Name))
		}
		parsed, parseErr := parseFilterValue(field.Type, operator, values[key])
		if parseErr != nil {
			return Params{}, soroerrors.InvalidQuery(fmt.Sprintf("invalid %s filter: %v", field.Name, parseErr))
		}
		params.Filters = append(params.Filters, Filter{Field: field, Operator: operator, Value: parsed})
	}
	return params, nil
}

func Apply(selectQuery *bun.SelectQuery, params Params, definition Definition) *bun.SelectQuery {
	for _, filter := range params.Filters {
		identifier := bun.Ident(filter.Field.Column)
		switch filter.Operator {
		case Eq:
			selectQuery = selectQuery.Where("? = ?", identifier, filter.Value)
		case Neq:
			selectQuery = selectQuery.Where("? <> ?", identifier, filter.Value)
		case Gt:
			selectQuery = selectQuery.Where("? > ?", identifier, filter.Value)
		case Gte:
			selectQuery = selectQuery.Where("? >= ?", identifier, filter.Value)
		case Lt:
			selectQuery = selectQuery.Where("? < ?", identifier, filter.Value)
		case Lte:
			selectQuery = selectQuery.Where("? <= ?", identifier, filter.Value)
		case In:
			selectQuery = selectQuery.Where("? IN (?)", identifier, bun.List(filter.Value))
		case Contains:
			selectQuery = selectQuery.Where("? ILIKE ? ESCAPE '!'", identifier, "%"+EscapeLike(filter.Value.(string))+"%")
		}
	}
	if params.Search != "" && len(definition.Searchable) > 0 {
		pattern := "%" + EscapeLike(params.Search) + "%"
		selectQuery = selectQuery.WhereGroup(" AND ", func(group *bun.SelectQuery) *bun.SelectQuery {
			for _, column := range definition.Searchable {
				group = group.WhereOr("? ILIKE ? ESCAPE '!'", bun.Ident(column), pattern)
			}
			return group
		})
	}
	for _, sortField := range params.Sort {
		direction := "ASC"
		if sortField.Descending {
			direction = "DESC"
		}
		selectQuery = selectQuery.OrderExpr("? "+direction, bun.Ident(sortField.Column))
	}
	return selectQuery
}

func ApplyPagination(selectQuery *bun.SelectQuery, params Params) *bun.SelectQuery {
	return selectQuery.Offset((params.Page - 1) * params.PerPage).Limit(params.PerPage)
}

func EscapeLike(value string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return replacer.Replace(value)
}

func parseFilterValue(valueType ValueType, operator Operator, rawValues []string) (any, error) {
	if operator == In {
		parts := make([]string, 0)
		for _, raw := range rawValues {
			parts = append(parts, strings.Split(raw, ",")...)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("value is required")
		}
		parsed := make([]any, 0, len(parts))
		for _, part := range parts {
			if part == "" {
				return nil, fmt.Errorf("in values cannot be empty")
			}
			value, err := parseScalar(valueType, part)
			if err != nil {
				return nil, err
			}
			parsed = append(parsed, value)
		}
		return parsed, nil
	}
	if len(rawValues) != 1 || rawValues[0] == "" {
		return nil, fmt.Errorf("exactly one non-empty value is required")
	}
	return parseScalar(valueType, rawValues[0])
}

func parseScalar(valueType ValueType, raw string) (any, error) {
	switch valueType {
	case String:
		return raw, nil
	case Bool:
		if raw != "true" && raw != "false" {
			return nil, fmt.Errorf("must be true or false")
		}
		return strconv.ParseBool(raw)
	case Int:
		return strconv.ParseInt(raw, 10, 64)
	case Float:
		return strconv.ParseFloat(raw, 64)
	case UUID:
		return uuid.Parse(raw)
	case Date:
		return time.Parse(time.DateOnly, raw)
	case Timestamp:
		return time.Parse(time.RFC3339, raw)
	default:
		return nil, fmt.Errorf("unsupported type %s", valueType)
	}
}

func parseSort(raw string, definitions []SortField) ([]Sort, error) {
	if raw == "" {
		return nil, soroerrors.InvalidQuery("sort cannot be empty")
	}
	allowed := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		allowed[definition.Name] = definition.Column
	}
	parts := strings.Split(raw, ",")
	result := make([]Sort, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		descending := strings.HasPrefix(part, "-")
		name := strings.TrimPrefix(part, "-")
		column, ok := allowed[name]
		if !ok || name == "" {
			return nil, soroerrors.InvalidQuery("unsupported sort " + name)
		}
		if _, exists := seen[name]; exists {
			return nil, soroerrors.InvalidQuery("duplicate sort " + name)
		}
		seen[name] = struct{}{}
		result = append(result, Sort{Field: name, Column: column, Descending: descending})
	}
	return result, nil
}

func resolveDefaultSort(defaults []Sort, definitions []SortField) []Sort {
	columns := make(map[string]string, len(definitions))
	for _, definition := range definitions {
		columns[definition.Name] = definition.Column
	}
	result := make([]Sort, 0, len(defaults))
	for _, configured := range defaults {
		configured.Column = columns[configured.Field]
		result = append(result, configured)
	}
	return result
}

func positiveInteger(name, raw string) (int, error) {
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 1 {
		return 0, soroerrors.InvalidQuery(name + " must be a positive integer")
	}
	return parsed, nil
}

func first(values url.Values, key string) (string, bool) {
	entries, present := values[key]
	if !present {
		return "", false
	}
	if len(entries) != 1 {
		return "", true
	}
	return entries[0], true
}

func knownType(valueType ValueType) bool {
	switch valueType {
	case String, Bool, Int, Float, UUID, Date, Timestamp:
		return true
	default:
		return false
	}
}

func operatorAllowed(valueType ValueType, operator Operator) bool {
	switch operator {
	case Eq, Neq, In:
		return true
	case Contains:
		return valueType == String
	case Gt, Gte, Lt, Lte:
		return valueType != Bool && valueType != UUID
	default:
		return false
	}
}

func containsOperator(operators []Operator, wanted Operator) bool {
	for _, operator := range operators {
		if operator == wanted {
			return true
		}
	}
	return false
}
