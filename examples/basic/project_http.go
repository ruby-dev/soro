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

type ProjectResponse struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Metadata    model.Metadata `json:"metadata"`
	AccountID   uuid.UUID      `json:"account_id"`
	OwnerID     *uuid.UUID     `json:"owner_id,omitempty"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type CreateProjectInput struct {
	Name        string         `json:"name" minLength:"1" maxLength:"255"`
	Description string         `json:"description,omitempty"`
	Metadata    model.Metadata `json:"metadata,omitempty"`
	AccountID   uuid.UUID      `json:"account_id"`
	OwnerID     *uuid.UUID     `json:"owner_id,omitempty"`
	Status      string         `json:"status" minLength:"1" maxLength:"64"`
}

type UpdateProjectInput struct {
	Name        *string         `json:"name,omitempty" minLength:"1" maxLength:"255"`
	Description *string         `json:"description,omitempty"`
	Metadata    *model.Metadata `json:"metadata,omitempty"`
	OwnerID     **uuid.UUID     `json:"owner_id,omitempty"`
	Status      *string         `json:"status,omitempty" minLength:"1" maxLength:"64"`
}

func NewProjectResource(projects *repository.Repository[Project]) (*api.Resource[Project, CreateProjectInput, UpdateProjectInput, ProjectResponse], error) {
	return api.NewResource(api.ResourceConfig[Project, CreateProjectInput, UpdateProjectInput, ProjectResponse]{
		Name:       "Projects",
		Repository: projects,
		Serializer: serializer.Func[Project, ProjectResponse](func(_ context.Context, project *Project) (ProjectResponse, error) {
			return ProjectResponse{
				ID: project.ID, Name: project.Name, Description: project.Description, Metadata: project.Metadata,
				AccountID: project.AccountID, OwnerID: project.OwnerID, Status: project.Status,
				CreatedAt: project.CreatedAt, UpdatedAt: project.UpdatedAt,
			}, nil
		}),
		CreateEntity: func(_ context.Context, input CreateProjectInput) (*Project, error) {
			return &Project{
				Base:      model.Base{Name: strings.TrimSpace(input.Name), Description: input.Description, Metadata: input.Metadata},
				AccountID: input.AccountID, OwnerID: input.OwnerID, Status: strings.TrimSpace(input.Status),
			}, nil
		},
		UpdateEntity: func(_ context.Context, project *Project, input UpdateProjectInput) error {
			if input.Name != nil {
				project.Name = strings.TrimSpace(*input.Name)
			}
			if input.Description != nil {
				project.Description = *input.Description
			}
			if input.Metadata != nil {
				project.Metadata = *input.Metadata
			}
			if input.OwnerID != nil {
				project.OwnerID = *input.OwnerID
			}
			if input.Status != nil {
				project.Status = strings.TrimSpace(*input.Status)
			}
			return nil
		},
		Query: query.Definition{
			Filters: []query.Field{
				{Name: "account_id", Column: "account_id", Type: query.UUID, Operators: []query.Operator{query.Eq, query.In}},
				{Name: "owner_id", Column: "owner_id", Type: query.UUID, Operators: []query.Operator{query.Eq, query.In}},
				{Name: "status", Column: "status", Type: query.String, Operators: []query.Operator{query.Eq, query.In}},
			},
			Searchable:  []string{"name", "description"},
			Sortable:    []query.SortField{{Name: "name", Column: "name"}, {Name: "created_at", Column: "created_at"}, {Name: "status", Column: "status"}},
			DefaultSort: []query.Sort{{Field: "created_at", Descending: true}},
		},
	})
}
