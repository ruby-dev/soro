package generate

import "testing"

func TestParseName(t *testing.T) {
	for input, expected := range map[string]Name{
		"User":          {Singular: "User", Snake: "user", Plural: "Users", Table: "users"},
		"APIKey":        {Singular: "ApiKey", Snake: "api_key", Plural: "ApiKeys", Table: "api_keys"},
		"company":       {Singular: "Company", Snake: "company", Plural: "Companies", Table: "companies"},
		"PostalAddress": {Singular: "PostalAddress", Snake: "postal_address", Plural: "PostalAddresses", Table: "postal_addresses"},
	} {
		actual, err := ParseName(input)
		if err != nil {
			t.Fatalf("parse %q: %v", input, err)
		}
		if actual != expected {
			t.Fatalf("parse %q: got %#v, want %#v", input, actual, expected)
		}
	}
}

func TestParseFields(t *testing.T) {
	fields, err := ParseFields([]string{"email:string:unique:index", "active:bool:default=true", "account_id:uuid:null", "metadata_override:json:default={}"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 4 || !fields[0].Unique || !fields[0].Index || fields[1].GoType() != "bool" || fields[2].GoType() != "*uuid.UUID" {
		t.Fatalf("unexpected fields: %#v", fields)
	}
	if value, _ := fields[1].SQLDefault(); value != "TRUE" {
		t.Fatalf("unexpected default %q", value)
	}
}

func TestParseFieldRejectsInvalidInput(t *testing.T) {
	for _, specification := range []string{"Email:string", "name", "name:wat", "name:string:null:unique", "active:bool:default=yes", "id:uuid:default=random", "name:string:index:index", "two__words:string"} {
		if _, err := ParseField(specification); err == nil {
			t.Fatalf("expected %q to fail", specification)
		}
	}
	if _, err := ParseFields([]string{"name:string", "name:text"}); err == nil {
		t.Fatal("expected duplicate fields to fail")
	}
	if _, err := ParseFields([]string{"id:uuid"}); err == nil {
		t.Fatal("expected base field to fail")
	}
}
