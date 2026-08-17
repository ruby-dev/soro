package basic

import (
	"context"
	"strings"
	"time"

	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/jobs"
	"github.com/datasoro/soro/model"
	"github.com/datasoro/soro/query"
	"github.com/datasoro/soro/repository"
	"github.com/datasoro/soro/serializer"
	"github.com/google/uuid"
)

type UserResponse struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Active    bool       `json:"active"`
	AccountID *uuid.UUID `json:"account_id,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type CreateUserInput struct {
	Name      string     `json:"name" minLength:"1" maxLength:"255"`
	Email     string     `json:"email" format:"email"`
	Active    bool       `json:"active"`
	AccountID *uuid.UUID `json:"account_id,omitempty"`
}

type UpdateUserInput struct {
	Name      *string     `json:"name,omitempty" minLength:"1" maxLength:"255"`
	Email     *string     `json:"email,omitempty" format:"email"`
	Active    *bool       `json:"active,omitempty"`
	AccountID **uuid.UUID `json:"account_id,omitempty"`
}

func NewUserResource(dbRepository *repository.Repository[User]) (*api.Resource[User, CreateUserInput, UpdateUserInput, UserResponse], error) {
	return newUserResource(dbRepository, nil)
}

func NewUserResourceWithJobs(dbRepository *repository.Repository[User], jobClient *jobs.Client) (*api.Resource[User, CreateUserInput, UpdateUserInput, UserResponse], error) {
	return newUserResource(dbRepository, jobClient)
}

func newUserResource(dbRepository *repository.Repository[User], jobClient *jobs.Client) (*api.Resource[User, CreateUserInput, UpdateUserInput, UserResponse], error) {
	return api.NewResource(api.ResourceConfig[User, CreateUserInput, UpdateUserInput, UserResponse]{
		Name:       "Users",
		Repository: dbRepository,
		Serializer: serializer.Func[User, UserResponse](func(_ context.Context, user *User) (UserResponse, error) {
			return UserResponse{
				ID: user.ID, Name: user.Name, Email: user.Email, Active: user.Active, AccountID: user.AccountID,
				CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
			}, nil
		}),
		CreateEntity: func(_ context.Context, input CreateUserInput) (*User, error) {
			return &User{Email: input.Email, Active: input.Active, AccountID: input.AccountID, Base: baseWithName(input.Name)}, nil
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
			if input.AccountID != nil {
				user.AccountID = *input.AccountID
			}
			return nil
		},
		After: func(ctx context.Context, action api.Action, user *User) error {
			if action != api.Create || jobClient == nil {
				return nil
			}
			_, err := jobClient.Enqueue(ctx, SendWelcomeEmail{UserID: user.ID}, jobs.Queue("mailers"))
			return err
		},
		Query: query.Definition{
			Filters: []query.Field{
				{Name: "active", Column: "active", Type: query.Bool, Operators: []query.Operator{query.Eq, query.Neq}},
				{Name: "account_id", Column: "account_id", Type: query.UUID, Operators: []query.Operator{query.Eq, query.In}},
			},
			Searchable:  []string{"name", "email"},
			Sortable:    []query.SortField{{Name: "name", Column: "name"}, {Name: "created_at", Column: "created_at"}, {Name: "email", Column: "email"}},
			DefaultSort: []query.Sort{{Field: "created_at", Descending: true}},
		},
	})
}

func baseWithName(name string) model.Base {
	return model.Base{Name: strings.TrimSpace(name)}
}
