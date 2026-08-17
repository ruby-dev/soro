// Package basic is a compiling Phase 1 example application model.
package basic

import (
	"context"
	"strings"

	"github.com/datasoro/soro/lifecycle"
	"github.com/datasoro/soro/model"
	"github.com/google/uuid"
)

type User struct {
	model.Base
	Email     string     `bun:"email,notnull" json:"email" validate:"required,email"`
	Active    bool       `bun:"active,notnull,default:true" json:"active"`
	AccountID *uuid.UUID `bun:"account_id,nullzero" json:"account_id,omitempty"`

	EmailChanged bool `bun:"-" json:"-"`
}

func (user *User) BeforeCreate(_ context.Context, _ *lifecycle.Context) error {
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	return nil
}

func (user *User) AfterUpdate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.EmailChanged = lifecycleContext.Changes.Changed("Email")
	return nil
}
