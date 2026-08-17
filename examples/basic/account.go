package basic

import (
	"context"
	"strings"

	"github.com/datasoro/soro/lifecycle"
	"github.com/datasoro/soro/model"
)

type Account struct {
	model.Base
	Slug string `bun:"slug,notnull" json:"slug" validate:"required,max=255"`
}

func (account *Account) BeforeCreate(_ context.Context, _ *lifecycle.Context) error {
	account.Slug = strings.ToLower(strings.TrimSpace(account.Slug))
	return nil
}
