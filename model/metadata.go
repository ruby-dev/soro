package model

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
)

// Metadata is arbitrary JSON object data persisted as PostgreSQL JSONB.
type Metadata map[string]any

func (m Metadata) Get(key string) (any, bool) {
	value, ok := m[key]
	return value, ok
}

func (m Metadata) Has(key string) bool {
	_, ok := m[key]
	return ok
}

func (m *Metadata) Set(key string, value any) {
	if *m == nil {
		*m = Metadata{}
	}
	(*m)[key] = value
}

func (m Metadata) Delete(key string) { delete(m, key) }

func (m Metadata) GetString(key string) (string, error) {
	value, ok := m[key]
	if !ok {
		return "", missingMetadataKey(key)
	}
	result, ok := value.(string)
	if !ok {
		return "", metadataTypeError(key, "string", value)
	}
	return result, nil
}

func (m Metadata) GetBool(key string) (bool, error) {
	value, ok := m[key]
	if !ok {
		return false, missingMetadataKey(key)
	}
	result, ok := value.(bool)
	if !ok {
		return false, metadataTypeError(key, "bool", value)
	}
	return result, nil
}

func (m Metadata) GetInt(key string) (int, error) {
	value, ok := m[key]
	if !ok {
		return 0, missingMetadataKey(key)
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int8:
		return int(typed), nil
	case int16:
		return int(typed), nil
	case int32:
		return int(typed), nil
	case int64:
		if int64(int(typed)) != typed {
			return 0, fmt.Errorf("metadata %q integer overflows int", key)
		}
		return int(typed), nil
	case float64:
		if math.Trunc(typed) != typed || typed > float64(maxInt()) || typed < float64(minInt()) {
			return 0, metadataTypeError(key, "integer", value)
		}
		return int(typed), nil
	case json.Number:
		integer, err := typed.Int64()
		if err != nil || int64(int(integer)) != integer {
			return 0, metadataTypeError(key, "integer", value)
		}
		return int(integer), nil
	default:
		return 0, metadataTypeError(key, "integer", value)
	}
}

func (m Metadata) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(encoded), nil
}

func (m *Metadata) Scan(src any) error {
	if m == nil {
		return fmt.Errorf("scan metadata into nil receiver")
	}
	if src == nil {
		*m = Metadata{}
		return nil
	}
	var data []byte
	switch value := src.(type) {
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return fmt.Errorf("scan metadata from %T", src)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded Metadata
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}
	if decoded == nil {
		decoded = Metadata{}
	}
	*m = decoded
	return nil
}

func missingMetadataKey(key string) error {
	return fmt.Errorf("metadata key %q does not exist", key)
}

func metadataTypeError(key, want string, value any) error {
	return fmt.Errorf("metadata key %q must be %s, got %T", key, want, value)
}

func maxInt() int { return int(^uint(0) >> 1) }
func minInt() int { return -maxInt() - 1 }
