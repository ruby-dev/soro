// Package lifecycle defines Soro's model operation hooks and change context.
package lifecycle

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type Operation string

const (
	OperationCreate      Operation = "create"
	OperationUpdate      Operation = "update"
	OperationDelete      Operation = "delete"
	OperationRestore     Operation = "restore"
	OperationForceDelete Operation = "force_delete"
)

type Stage string

const (
	BeforeValidate    Stage = "before_validate"
	AfterValidate     Stage = "after_validate"
	BeforeSave        Stage = "before_save"
	AfterSave         Stage = "after_save"
	BeforeCreate      Stage = "before_create"
	AfterCreate       Stage = "after_create"
	BeforeUpdate      Stage = "before_update"
	AfterUpdate       Stage = "after_update"
	BeforeDelete      Stage = "before_delete"
	AfterDelete       Stage = "after_delete"
	BeforeRestore     Stage = "before_restore"
	AfterRestore      Stage = "after_restore"
	BeforeForceDelete Stage = "before_force_delete"
	AfterForceDelete  Stage = "after_force_delete"
	AfterCommit       Stage = "after_commit"
	AfterRollback     Stage = "after_rollback"
)

type Context struct {
	Operation Operation
	Changes   Changes
	Tx        bun.IDB
	ActorID   *uuid.UUID
	Metadata  map[string]any
}

type BeforeValidateHook interface {
	BeforeValidate(context.Context, *Context) error
}
type AfterValidateHook interface {
	AfterValidate(context.Context, *Context) error
}
type BeforeSaveHook interface {
	BeforeSave(context.Context, *Context) error
}
type AfterSaveHook interface {
	AfterSave(context.Context, *Context) error
}
type BeforeCreateHook interface {
	BeforeCreate(context.Context, *Context) error
}
type AfterCreateHook interface {
	AfterCreate(context.Context, *Context) error
}
type BeforeUpdateHook interface {
	BeforeUpdate(context.Context, *Context) error
}
type AfterUpdateHook interface {
	AfterUpdate(context.Context, *Context) error
}
type BeforeDeleteHook interface {
	BeforeDelete(context.Context, *Context) error
}
type AfterDeleteHook interface {
	AfterDelete(context.Context, *Context) error
}
type BeforeRestoreHook interface {
	BeforeRestore(context.Context, *Context) error
}
type AfterRestoreHook interface {
	AfterRestore(context.Context, *Context) error
}
type BeforeForceDeleteHook interface {
	BeforeForceDelete(context.Context, *Context) error
}
type AfterForceDeleteHook interface {
	AfterForceDelete(context.Context, *Context) error
}
type AfterCommitHook interface {
	AfterCommit(context.Context, *Context) error
}
type AfterRollbackHook interface {
	AfterRollback(context.Context, *Context) error
}
