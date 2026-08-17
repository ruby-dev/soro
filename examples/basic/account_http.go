package basic

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ruby-dev/soro/api"
	"github.com/ruby-dev/soro/query"
	"github.com/ruby-dev/soro/repository"
	"github.com/ruby-dev/soro/serializer"
)

type AccountResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateAccountInput struct {
	Name string `json:"name" minLength:"1" maxLength:"255"`
	Slug string `json:"slug" minLength:"1" maxLength:"255"`
}

type UpdateAccountInput struct {
	Name *string `json:"name,omitempty" minLength:"1" maxLength:"255"`
	Slug *string `json:"slug,omitempty" minLength:"1" maxLength:"255"`
}

func NewAccountResource(accounts *repository.Repository[Account]) (*api.Resource[Account, CreateAccountInput, UpdateAccountInput, AccountResponse], error) {
	return api.NewResource(api.ResourceConfig[Account, CreateAccountInput, UpdateAccountInput, AccountResponse]{
		Name:       "Accounts",
		Repository: accounts,
		Serializer: serializer.Func[Account, AccountResponse](func(_ context.Context, account *Account) (AccountResponse, error) {
			return AccountResponse{ID: account.ID, Name: account.Name, Slug: account.Slug, CreatedAt: account.CreatedAt, UpdatedAt: account.UpdatedAt}, nil
		}),
		CreateEntity: func(_ context.Context, input CreateAccountInput) (*Account, error) {
			return &Account{Base: baseWithName(input.Name), Slug: input.Slug}, nil
		},
		UpdateEntity: func(_ context.Context, account *Account, input UpdateAccountInput) error {
			if input.Name != nil {
				account.Name = strings.TrimSpace(*input.Name)
			}
			if input.Slug != nil {
				account.Slug = strings.ToLower(strings.TrimSpace(*input.Slug))
			}
			return nil
		},
		Query: query.Definition{
			Filters:     []query.Field{{Name: "slug", Column: "slug", Type: query.String, Operators: []query.Operator{query.Eq}}},
			Searchable:  []string{"name", "slug"},
			Sortable:    []query.SortField{{Name: "name", Column: "name"}, {Name: "created_at", Column: "created_at"}, {Name: "slug", Column: "slug"}},
			DefaultSort: []query.Sort{{Field: "created_at", Descending: true}},
		},
	})
}
