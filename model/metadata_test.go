package model_test

import (
	"encoding/json"
	"testing"

	"github.com/datasoro/soro/model"
)

func TestMetadataTypedAccess(t *testing.T) {
	metadata := model.Metadata{"name": "Soro", "active": true, "count": json.Number("4")}

	if value, err := metadata.GetString("name"); err != nil || value != "Soro" {
		t.Fatalf("GetString = %q, %v", value, err)
	}
	if value, err := metadata.GetBool("active"); err != nil || !value {
		t.Fatalf("GetBool = %t, %v", value, err)
	}
	if value, err := metadata.GetInt("count"); err != nil || value != 4 {
		t.Fatalf("GetInt = %d, %v", value, err)
	}
	if _, err := metadata.GetString("active"); err == nil {
		t.Fatal("expected invalid type error")
	}
}

func TestMetadataScanAndValue(t *testing.T) {
	var metadata model.Metadata
	if err := metadata.Scan([]byte(`{"count":3,"nested":{"ok":true}}`)); err != nil {
		t.Fatal(err)
	}
	if count, err := metadata.GetInt("count"); err != nil || count != 3 {
		t.Fatalf("count = %d, %v", count, err)
	}
	value, err := metadata.Value()
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value.(string)), &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataSetInitializesNilMap(t *testing.T) {
	var metadata model.Metadata
	metadata.Set("ready", true)
	if ready, err := metadata.GetBool("ready"); err != nil || !ready {
		t.Fatalf("ready = %t, %v", ready, err)
	}
}
