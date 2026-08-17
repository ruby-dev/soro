package basic

import (
	"context"
	"fmt"

	soroerrors "github.com/datasoro/soro/errors"
	"github.com/datasoro/soro/factory"
	"github.com/datasoro/soro/model"
	"github.com/datasoro/soro/repository"
)

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
