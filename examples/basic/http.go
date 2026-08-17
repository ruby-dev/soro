package basic

import (
	"context"
	"strings"
	"time"

	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/model"
	"github.com/datasoro/soro/query"
	"github.com/datasoro/soro/repository"
	"github.com/datasoro/soro/serializer"
	"github.com/google/uuid"
)

type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateUserInput struct {
	Name   string `json:"name" minLength:"1" maxLength:"255"`
	Email  string `json:"email" format:"email"`
	Active bool   `json:"active"`
}

type UpdateUserInput struct {
	Name   *string `json:"name,omitempty" minLength:"1" maxLength:"255"`
	Email  *string `json:"email,omitempty" format:"email"`
	Active *bool   `json:"active,omitempty"`
}

func NewUserResource(dbRepository *repository.Repository[User]) (*api.Resource[User, CreateUserInput, UpdateUserInput, UserResponse], error) {
	return api.NewResource(api.ResourceConfig[User, CreateUserInput, UpdateUserInput, UserResponse]{
		Name:       "Users",
		Repository: dbRepository,
		Serializer: serializer.Func[User, UserResponse](func(_ context.Context, user *User) (UserResponse, error) {
			return UserResponse{
				ID: user.ID, Name: user.Name, Email: user.Email, Active: user.Active,
				CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
			}, nil
		}),
		CreateEntity: func(_ context.Context, input CreateUserInput) (*User, error) {
			return &User{Email: input.Email, Active: input.Active, Base: baseWithName(input.Name)}, nil
		},
		UpdateEntity: func(_ context.Context, user *User, input UpdateUserInput) error {
			if input.Name != nil {
				user.Name = strings.TrimSpace(*input.Name)
			}
			if input.Email != nil {
				user.Email = strings.ToLower(strings.TrimSpace(*input.Email))
			}
			if input.Active != nil {
				user.Active = *input.Active
			}
			return nil
		},
		Query: query.Definition{
			Filters:     []query.Field{{Name: "active", Column: "active", Type: query.Bool, Operators: []query.Operator{query.Eq, query.Neq}}},
			Searchable:  []string{"name", "email"},
			Sortable:    []query.SortField{{Name: "name", Column: "name"}, {Name: "created_at", Column: "created_at"}, {Name: "email", Column: "email"}},
			DefaultSort: []query.Sort{{Field: "created_at", Descending: true}},
		},
	})
}

func baseWithName(name string) model.Base {
	return model.Base{Name: strings.TrimSpace(name)}
}
