package basic

import (
	"context"
	"fmt"

	"github.com/datasoro/soro/database"
	soroerrors "github.com/datasoro/soro/errors"
	"github.com/datasoro/soro/factory"
	"github.com/datasoro/soro/model"
	"github.com/datasoro/soro/repository"
	"github.com/google/uuid"
)

func NewAccountFactory(accounts *repository.Repository[Account]) (*factory.Factory[Account], error) {
	if accounts == nil {
		return nil, fmt.Errorf("account repository is required")
	}
	return factory.New(func(sequence uint64) *Account {
		return &Account{
			Base: model.Base{Name: fmt.Sprintf("Account %d", sequence), Metadata: model.Metadata{"source": "factory"}},
			Slug: fmt.Sprintf("account-%d", sequence),
		}
	}, func(ctx context.Context, account *Account) error { return accounts.Create(ctx, account) })
}

func NewUserFactory(users *repository.Repository[User]) (*factory.Factory[User], error) {
	if users == nil {
		return nil, fmt.Errorf("user repository is required")
	}
	return factory.New(func(sequence uint64) *User {
		return &User{
			Base: model.Base{
				Name:     fmt.Sprintf("User %d", sequence),
				Metadata: model.Metadata{"source": "factory"},
			},
			Email:  fmt.Sprintf("user-%d@example.com", sequence),
			Active: true,
		}
	}, func(ctx context.Context, user *User) error { return users.Create(ctx, user) })
}

// Seed inserts one predictable development user and is safe to run repeatedly.
func Seed(ctx context.Context, users *repository.Repository[User]) error {
	const email = "demo@example.com"
	if _, err := users.FindBy(ctx, "Email", email); err == nil {
		return nil
	} else if !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		return err
	}
	usersFactory, err := NewUserFactory(users)
	if err != nil {
		return err
	}
	_, err = usersFactory.Create(ctx, func(user *User) {
		user.Name = "Demo User"
		user.Email = email
		user.Metadata.Set("seed", true)
	})
	return err
}

func NewProjectFactory(projects *repository.Repository[Project], accountID uuid.UUID, ownerID *uuid.UUID) (*factory.Factory[Project], error) {
	if projects == nil {
		return nil, fmt.Errorf("project repository is required")
	}
	if accountID == uuid.Nil {
		return nil, fmt.Errorf("project factory account ID is required")
	}
	return factory.New(func(sequence uint64) *Project {
		return &Project{
			Base: model.Base{
				Name:     fmt.Sprintf("Project %d", sequence),
				Metadata: model.Metadata{"source": "factory"},
			},
			AccountID: accountID,
			OwnerID:   ownerID,
			Status:    "active",
		}
	}, func(ctx context.Context, project *Project) error { return projects.Create(ctx, project) })
}

// SeedApplication creates an Account/User/Project graph and is idempotent.
func SeedApplication(ctx context.Context, db *database.DB) error {
	accounts := repository.New[Account](db)
	users := repository.New[User](db)
	projects := repository.New[Project](db)

	account, err := accounts.FindBy(ctx, "Slug", "datasoro-demo")
	if soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		accountsFactory, factoryErr := NewAccountFactory(accounts)
		if factoryErr != nil {
			return factoryErr
		}
		account, err = accountsFactory.Create(ctx, func(account *Account) {
			account.Name = "DataSoro Demo"
			account.Slug = "datasoro-demo"
			account.Metadata.Set("seed", true)
		})
	}
	if err != nil {
		return err
	}

	user, err := users.FindBy(ctx, "Email", "demo@example.com")
	if soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		usersFactory, factoryErr := NewUserFactory(users)
		if factoryErr != nil {
			return factoryErr
		}
		user, err = usersFactory.Create(ctx, func(user *User) {
			user.Name = "Demo User"
			user.Email = "demo@example.com"
			user.AccountID = &account.ID
			user.Metadata.Set("seed", true)
		})
	}
	if err != nil {
		return err
	}

	if _, err := projects.FindBy(ctx, "Name", "Soro"); err == nil {
		return nil
	} else if !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		return err
	}
	projectsFactory, err := NewProjectFactory(projects, account.ID, &user.ID)
	if err != nil {
		return err
	}
	_, err = projectsFactory.Create(ctx, func(project *Project) {
		project.Name = "Soro"
		project.Description = "Convention-driven Go application framework"
		project.Metadata.Set("language", "go")
	})
	return err
}
