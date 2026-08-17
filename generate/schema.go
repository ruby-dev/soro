// Package generate implements Soro's deterministic application and component generators.
package generate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	goNamePattern    = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	fieldNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	modulePattern    = regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`)
)

type Name struct {
	Singular string
	Snake    string
	Plural   string
	Table    string
}

func ParseName(raw string) (Name, error) {
	words := wordsOf(raw)
	if len(words) == 0 {
		return Name{}, fmt.Errorf("generator name is required")
	}
	singular := exportedName(words)
	if !goNamePattern.MatchString(singular) {
		return Name{}, fmt.Errorf("invalid generator name %q", raw)
	}
	snake := strings.Join(words, "_")
	pluralSnake := pluralize(snake)
	return Name{Singular: singular, Snake: snake, Plural: exportedName(wordsOf(pluralSnake)), Table: pluralSnake}, nil
}

func exportedName(words []string) string {
	var exported strings.Builder
	for _, word := range words {
		exported.WriteString(strings.ToUpper(word[:1]))
		exported.WriteString(word[1:])
	}
	return exported.String()
}

func wordsOf(raw string) []string {
	raw = strings.TrimSpace(raw)
	var result []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			result = append(result, strings.ToLower(string(current)))
			current = nil
		}
	}
	characters := []rune(raw)
	for index, character := range characters {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			flush()
			continue
		}
		if len(current) > 0 && unicode.IsUpper(character) {
			previous := characters[index-1]
			nextLower := index+1 < len(characters) && unicode.IsLower(characters[index+1])
			if unicode.IsLower(previous) || unicode.IsDigit(previous) || nextLower {
				flush()
			}
		}
		current = append(current, unicode.ToLower(character))
	}
	flush()
	return result
}

func pluralize(value string) string {
	if strings.HasSuffix(value, "y") && len(value) > 1 && !strings.ContainsRune("aeiou", rune(value[len(value)-2])) {
		return strings.TrimSuffix(value, "y") + "ies"
	}
	for _, suffix := range []string{"s", "x", "z", "ch", "sh"} {
		if strings.HasSuffix(value, suffix) {
			return value + "es"
		}
	}
	return value + "s"
}

type FieldType string

const (
	String FieldType = "string"
	Text   FieldType = "text"
	Bool   FieldType = "bool"
	UUID   FieldType = "uuid"
	Int    FieldType = "int"
	Float  FieldType = "float"
	Time   FieldType = "time"
	JSON   FieldType = "json"
)

type Field struct {
	Name       string
	GoName     string
	Type       FieldType
	Index      bool
	Unique     bool
	Nullable   bool
	Default    string
	HasDefault bool
}

func ParseFields(specifications []string) ([]Field, error) {
	fields := make([]Field, 0, len(specifications))
	seen := make(map[string]struct{}, len(specifications))
	seenGo := make(map[string]struct{}, len(specifications))
	baseFields := map[string]struct{}{
		"id": {}, "name": {}, "description": {}, "metadata": {}, "created_at": {}, "updated_at": {},
		"deleted_at": {}, "created_by": {}, "updated_by": {}, "deleted_by": {},
	}
	for _, specification := range specifications {
		field, err := ParseField(specification)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[field.Name]; exists {
			return nil, fmt.Errorf("duplicate field %q", field.Name)
		}
		if _, reserved := baseFields[field.Name]; reserved {
			return nil, fmt.Errorf("field %q is provided by model.Base", field.Name)
		}
		if _, exists := seenGo[field.GoName]; exists {
			return nil, fmt.Errorf("field %q produces duplicate Go name %s", field.Name, field.GoName)
		}
		seen[field.Name] = struct{}{}
		seenGo[field.GoName] = struct{}{}
		fields = append(fields, field)
	}
	return fields, nil
}

func ParseField(specification string) (Field, error) {
	parts := strings.Split(specification, ":")
	if len(parts) < 2 || !fieldNamePattern.MatchString(parts[0]) || strings.Contains(parts[0], "__") {
		return Field{}, fmt.Errorf("field %q must use name:type[:option...]", specification)
	}
	field := Field{Name: parts[0], Type: FieldType(parts[1])}
	name, err := ParseName(field.Name)
	if err != nil {
		return Field{}, err
	}
	field.GoName = name.Singular
	switch field.Type {
	case String, Text, Bool, UUID, Int, Float, Time, JSON:
	default:
		return Field{}, fmt.Errorf("field %s has unsupported type %q", field.Name, field.Type)
	}
	seen := make(map[string]struct{})
	for _, option := range parts[2:] {
		key := option
		if before, _, found := strings.Cut(option, "="); found {
			key = before
		}
		if _, exists := seen[key]; exists {
			return Field{}, fmt.Errorf("field %s repeats option %q", field.Name, key)
		}
		seen[key] = struct{}{}
		switch key {
		case "index":
			field.Index = true
		case "unique":
			field.Unique = true
		case "null":
			field.Nullable = true
		case "default":
			value, found := strings.CutPrefix(option, "default=")
			if !found || value == "" {
				return Field{}, fmt.Errorf("field %s default requires a value", field.Name)
			}
			field.Default, field.HasDefault = value, true
		default:
			return Field{}, fmt.Errorf("field %s has unsupported option %q", field.Name, option)
		}
	}
	if field.Nullable && (field.Unique || field.HasDefault) {
		return Field{}, fmt.Errorf("field %s cannot combine null with unique or default", field.Name)
	}
	if field.HasDefault {
		if _, err := field.SQLDefault(); err != nil {
			return Field{}, err
		}
	}
	return field, nil
}

func (field Field) GoType() string {
	value := map[FieldType]string{String: "string", Text: "string", Bool: "bool", UUID: "uuid.UUID", Int: "int64", Float: "float64", Time: "time.Time", JSON: "model.Metadata"}[field.Type]
	if field.Nullable {
		return "*" + value
	}
	return value
}

func (field Field) CreateGoType() string {
	if field.HasDefault {
		return "*" + field.GoType()
	}
	return field.GoType()
}

func (field Field) GoDefault() string {
	switch field.Type {
	case Bool, Int, Float:
		return field.Default
	case Time:
		return "time.Now().UTC()"
	case JSON:
		return "model.Metadata{}"
	default:
		return strconv.Quote(field.Default)
	}
}

func (field Field) SQLType() string {
	return map[FieldType]string{String: "VARCHAR(255)", Text: "TEXT", Bool: "BOOLEAN", UUID: "UUID", Int: "BIGINT", Float: "DOUBLE PRECISION", Time: "TIMESTAMPTZ", JSON: "JSONB"}[field.Type]
}

func (field Field) SQLDefault() (string, error) {
	if !field.HasDefault {
		return "", nil
	}
	switch field.Type {
	case Bool:
		if field.Default != "true" && field.Default != "false" {
			return "", fmt.Errorf("field %s boolean default must be true or false", field.Name)
		}
		return strings.ToUpper(field.Default), nil
	case Int:
		if _, err := strconv.ParseInt(field.Default, 10, 64); err != nil {
			return "", fmt.Errorf("field %s integer default is invalid", field.Name)
		}
		return field.Default, nil
	case Float:
		if _, err := strconv.ParseFloat(field.Default, 64); err != nil {
			return "", fmt.Errorf("field %s float default is invalid", field.Name)
		}
		return field.Default, nil
	case Time:
		if strings.ToLower(field.Default) != "now" {
			return "", fmt.Errorf("field %s time default only supports now", field.Name)
		}
		return "NOW()", nil
	case UUID:
		return "", fmt.Errorf("field %s UUID defaults are not supported", field.Name)
	case JSON:
		if field.Default != "{}" {
			return "", fmt.Errorf("field %s JSON default only supports {}", field.Name)
		}
		return "'{}'::jsonb", nil
	default:
		if strings.ContainsAny(field.Default, "\x00\n\r") {
			return "", fmt.Errorf("field %s string default contains invalid characters", field.Name)
		}
		return "'" + strings.ReplaceAll(field.Default, "'", "''") + "'", nil
	}
}

func ValidateModule(module string) error {
	if module == "" || !modulePattern.MatchString(module) || strings.Contains(module, "..") || strings.HasPrefix(module, "/") || strings.HasSuffix(module, "/") {
		return fmt.Errorf("invalid Go module path %q", module)
	}
	return nil
}
