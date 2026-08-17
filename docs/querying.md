# Resource querying

List endpoints support page pagination and resource-defined filters, search, and sorting. Every public field maps to a fixed database column:

```go
Query: query.Definition{
	Filters: []query.Field{
		{Name: "active", Column: "active", Type: query.Bool,
			Operators: []query.Operator{query.Eq, query.Neq}},
	},
	Searchable: []string{"name", "email"},
	Sortable: []query.SortField{
		{Name: "created_at", Column: "created_at"},
		{Name: "email", Column: "email"},
	},
	DefaultSort: []query.Sort{{Field: "created_at", Descending: true}},
}
```

Example requests:

```text
GET /api/v1/users?page=2&per_page=25
GET /api/v1/users?search=ward
GET /api/v1/users?filter[active]=true
GET /api/v1/users?filter[active][neq]=false
GET /api/v1/users?sort=-created_at,email
```

Defaults are page 1, 25 records per page, and a maximum of 100. The response contains `page`, `per_page`, `total`, and `pages`. The total is a separate filtered count query.

Supported filter operators are `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, and `contains`; valid operators depend on the declared value type. Supported types are string, bool, integer, float, UUID, date, and timestamp. `in` accepts comma-separated values.

Unknown parameters, fields, operators, sorts, repeated scalar values, and invalid typed values return `invalid_query`. Client parameter names are never interpolated into SQL. `contains` and search treat `%` and `_` literally by escaping PostgreSQL `ILIKE` patterns.

