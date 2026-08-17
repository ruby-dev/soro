package query_test

import (
	"net/url"
	"testing"

	"github.com/datasoro/soro/query"
)

func BenchmarkParse(b *testing.B) {
	definition := testDefinition()
	values := url.Values{
		"page": {"2"}, "per_page": {"25"}, "search": {"dustin"},
		"filter[active]": {"true"}, "sort": {"-created_at,name"},
	}
	for b.Loop() {
		params, err := query.Parse(values, definition)
		if err != nil || params.Page != 2 {
			b.Fatalf("Parse() params=%#v err=%v", params, err)
		}
	}
}
