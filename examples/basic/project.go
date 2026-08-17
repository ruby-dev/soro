package basic

import (
	"github.com/google/uuid"
	"github.com/ruby-dev/soro/model"
)

type Project struct {
	model.Base
	AccountID uuid.UUID  `bun:"account_id,notnull" json:"account_id" validate:"required"`
	OwnerID   *uuid.UUID `bun:"owner_id,nullzero" json:"owner_id,omitempty"`
	Status    string     `bun:"status,notnull" json:"status" validate:"required,max=64"`
}
