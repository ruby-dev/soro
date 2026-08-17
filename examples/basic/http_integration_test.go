package basic_test

import (
	"net/http"
	"testing"

	"github.com/datasoro/soro"
	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/repository"
	"github.com/datasoro/soro/testutil"
)

func TestAccountUserProjectAPI(t *testing.T) {
	app := sorotest.New(t,
		sorotest.WithMigrations(basic.Migrations...),
		sorotest.Setup(func(app *soro.App) error {
			accounts, err := basic.NewAccountResource(repository.New[basic.Account](app.DB))
			if err != nil {
				return err
			}
			users, err := basic.NewUserResource(repository.New[basic.User](app.DB))
			if err != nil {
				return err
			}
			projects, err := basic.NewProjectResource(repository.New[basic.Project](app.DB))
			if err != nil {
				return err
			}
			return app.API.Version("v1", func(router *api.Router) {
				for path, resource := range map[string]api.Registrar{
					"/accounts": accounts,
					"/users":    users,
					"/projects": projects,
				} {
					if registerErr := router.Resource(path, resource); registerErr != nil {
						t.Fatal(registerErr)
					}
				}
			})
		}),
	)

	account := createAccount(t, app)
	user := createRelatedUser(t, app, account.Data.ID.String())
	project := createProject(t, app, account.Data.ID.String(), user.Data.ID.String())
	if project.Data.AccountID != account.Data.ID || project.Data.OwnerID == nil || *project.Data.OwnerID != user.Data.ID {
		t.Fatalf("project relationship response = %#v", project.Data)
	}
	response, err := app.Client().Request(t.Context(), http.MethodGet,
		"/api/v1/projects?filter[account_id]="+account.Data.ID.String()+"&search=soro&sort=-created_at", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("project list status=%d body=%s", response.StatusCode, response.Body)
	}
	var list struct {
		Data []basic.ProjectResponse `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := response.Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 1 || list.Meta.Total != 1 || list.Data[0].ID != project.Data.ID {
		t.Fatalf("project list = %#v", list)
	}
}

type accountEnvelope struct {
	Data basic.AccountResponse `json:"data"`
}
type userEnvelope struct {
	Data basic.UserResponse `json:"data"`
}
type projectEnvelope struct {
	Data basic.ProjectResponse `json:"data"`
}

func createAccount(t *testing.T, app *sorotest.App) accountEnvelope {
	t.Helper()
	response, err := app.Client().Request(t.Context(), http.MethodPost, "/api/v1/accounts", map[string]any{"name": "DataSoro", "slug": "DATASORO"})
	if err != nil {
		t.Fatal(err)
	}
	var result accountEnvelope
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("account status=%d body=%s", response.StatusCode, response.Body)
	}
	if err := response.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func createRelatedUser(t *testing.T, app *sorotest.App, accountID string) userEnvelope {
	t.Helper()
	response, err := app.Client().Request(t.Context(), http.MethodPost, "/api/v1/users", map[string]any{
		"name": "Dustin", "email": "dustin@example.com", "active": true, "account_id": accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var result userEnvelope
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("user status=%d body=%s", response.StatusCode, response.Body)
	}
	if err := response.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func createProject(t *testing.T, app *sorotest.App, accountID, ownerID string) projectEnvelope {
	t.Helper()
	response, err := app.Client().Request(t.Context(), http.MethodPost, "/api/v1/projects", map[string]any{
		"name": "Soro", "description": "Go application framework", "metadata": map[string]any{"language": "go"},
		"account_id": accountID, "owner_id": ownerID, "status": "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	var result projectEnvelope
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("project status=%d body=%s", response.StatusCode, response.Body)
	}
	if err := response.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
