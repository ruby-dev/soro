package repository_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/datasoro/soro/auth"
	"github.com/datasoro/soro/database"
	soroerrors "github.com/datasoro/soro/errors"
	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/lifecycle"
	"github.com/datasoro/soro/migrate"
	"github.com/datasoro/soro/model"
	"github.com/datasoro/soro/repository"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func TestRepositoryPersistenceAndDeletionScopes(t *testing.T) {
	db := migratedDB(t)
	fixedTime := time.Date(2026, time.August, 16, 12, 30, 0, 0, time.FixedZone("test", -5*60*60))
	users := repository.New[basic.User](db, repository.WithClock(func() time.Time { return fixedTime }))
	user := &basic.User{
		Base:  model.Base{Name: "Dustin", Metadata: model.Metadata{"role": "admin", "count": 3}},
		Email: "USER@EXAMPLE.COM", Active: true,
	}
	actor := principalID(uuid.New())
	createContext := auth.WithPrincipal(t.Context(), actor)
	if err := users.Create(createContext, user); err != nil {
		t.Fatalf("%v (cause: %v)", err, causeOf(err))
	}
	if user.ID == uuid.Nil || user.ID.Version() != 7 {
		t.Fatalf("expected UUIDv7, got %s", user.ID)
	}
	if user.Email != "user@example.com" {
		t.Fatalf("BeforeCreate did not normalize email: %q", user.Email)
	}
	wantTime := fixedTime.UTC()
	if !user.CreatedAt.Equal(wantTime) || !user.UpdatedAt.Equal(wantTime) {
		t.Fatalf("timestamps = %s, %s; want %s", user.CreatedAt, user.UpdatedAt, wantTime)
	}
	if user.CreatedBy == nil || *user.CreatedBy != uuid.UUID(actor) || user.UpdatedBy == nil || *user.UpdatedBy != uuid.UUID(actor) {
		t.Fatalf("actor fields were not populated: created_by=%v updated_by=%v", user.CreatedBy, user.UpdatedBy)
	}

	found, err := users.Find(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if role, err := found.Metadata.GetString("role"); err != nil || role != "admin" {
		t.Fatalf("metadata role = %q, %v", role, err)
	}
	if count, err := found.Metadata.GetInt("count"); err != nil || count != 3 {
		t.Fatalf("metadata count = %d, %v", count, err)
	}
	byEmail, err := users.FindBy(t.Context(), "Email", "user@example.com")
	if err != nil || byEmail.ID != found.ID {
		t.Fatalf("FindBy = %#v, %v", byEmail, err)
	}
	first, err := users.First(t.Context())
	if err != nil || first.ID != found.ID {
		t.Fatalf("First = %#v, %v", first, err)
	}
	all, err := users.All(t.Context())
	if err != nil || len(all) != 1 || all[0].ID != found.ID {
		t.Fatalf("All = %#v, %v", all, err)
	}
	count, err := users.Count(t.Context())
	if err != nil || count != 1 {
		t.Fatalf("Count = %d, %v", count, err)
	}
	exists, err := users.Exists(t.Context(), found.ID)
	if err != nil || !exists {
		t.Fatalf("Exists = %t, %v", exists, err)
	}
	if _, err := users.FindBy(t.Context(), "email; DROP TABLE users", "x"); !soroerrors.IsCode(err, soroerrors.CodeInvalidArgument) {
		t.Fatalf("unsafe FindBy field was not rejected: %v", err)
	}

	found.Email = "changed@example.com"
	if err := users.Update(t.Context(), found); err != nil {
		t.Fatal(err)
	}
	if !found.EmailChanged {
		t.Fatal("AfterUpdate did not observe Email change")
	}
	if err := users.Delete(t.Context(), found.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Find(t.Context(), found.ID); !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		t.Fatalf("default scope should hide deletion, got %v", err)
	}
	if _, err := users.WithDeleted().Find(t.Context(), found.ID); err != nil {
		t.Fatalf("WithDeleted should find deletion: %v", err)
	}
	deleted, err := users.OnlyDeleted().Find(t.Context(), found.ID)
	if err != nil || deleted.DeletedAt == nil {
		t.Fatalf("OnlyDeleted = %#v, %v", deleted, err)
	}
	if err := users.Restore(t.Context(), found.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Find(t.Context(), found.ID); err != nil {
		t.Fatalf("restored user unavailable: %v", err)
	}
	if err := users.ForceDelete(t.Context(), found.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := users.WithDeleted().Find(t.Context(), found.ID); !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		t.Fatalf("force-deleted row still exists: %v", err)
	}
}

func TestPartialUniqueIndexPermitsReuseAfterSoftDelete(t *testing.T) {
	db := migratedDB(t)
	users := repository.New[basic.User](db)
	rollbacks := 0
	if err := db.Hooks().Register(basic.User{}, lifecycle.AfterRollback, 0,
		func(context.Context, any, *lifecycle.Context) error {
			rollbacks++
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	first := &basic.User{Email: "same@example.com"}
	if err := users.Create(t.Context(), first); err != nil {
		t.Fatalf("%v (cause: %v)", err, causeOf(err))
	}
	conflict := &basic.User{Email: "same@example.com"}
	if err := users.Create(t.Context(), conflict); !soroerrors.IsCode(err, soroerrors.CodeConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if rollbacks != 1 {
		t.Fatalf("database constraint failure invoked %d rollback hooks, want 1", rollbacks)
	}
	if err := users.Delete(t.Context(), first.ID); err != nil {
		t.Fatal(err)
	}
	if err := users.Create(t.Context(), conflict); err != nil {
		t.Fatalf("partial index rejected reused deleted email: %v", err)
	}
}

func TestContextCancellationPropagates(t *testing.T) {
	db := migratedDB(t)
	users := repository.New[basic.User](db)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := users.Find(ctx, uuid.New())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestLifecycleOrderingDirtyTrackingAndCommitTiming(t *testing.T) {
	db := migratedDB(t)
	events := []string{}
	users := repository.New[hookedUser](db)
	user := &hookedUser{Email: "first@example.com", Events: &events}

	if err := users.Transaction(t.Context(), func(ctx context.Context, txUsers *repository.Repository[hookedUser]) error {
		if err := txUsers.Create(ctx, user); err != nil {
			return err
		}
		if contains(events, "after_commit") {
			t.Fatal("AfterCommit ran before outer transaction committed")
		}
		return nil
	}); err != nil {
		t.Fatalf("%v (cause: %v)", err, causeOf(err))
	}
	assertEvents(t, events, []string{
		"before_validate", "validate", "after_validate", "before_save", "before_create",
		"after_create", "after_save", "after_commit",
	})

	events = events[:0]
	user.Email = "second@example.com"
	if err := users.Update(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	assertEvents(t, events, []string{
		"before_validate", "validate", "after_validate", "before_save", "before_update",
		"after_update", "after_save", "after_commit",
	})
	if user.OldEmail != "first@example.com" || user.NewEmail != "second@example.com" {
		t.Fatalf("dirty email = %q -> %q", user.OldEmail, user.NewEmail)
	}

	for _, operation := range []struct {
		name string
		run  func() error
		want []string
	}{
		{"delete", func() error {
			return users.Delete(t.Context(), user.ID, eventMetadata(&events))
		}, []string{"before_delete", "after_delete", "after_commit"}},
		{"restore", func() error {
			return users.Restore(t.Context(), user.ID, eventMetadata(&events))
		}, []string{"before_restore", "after_restore", "after_commit"}},
		{"force_delete", func() error {
			return users.ForceDelete(t.Context(), user.ID, eventMetadata(&events))
		}, []string{"before_force_delete", "after_force_delete", "after_commit"}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			events = events[:0]
			if err := operation.run(); err != nil {
				t.Fatal(err)
			}
			assertEvents(t, events, operation.want)
		})
	}
}

func TestLifecycleErrorRollsBackAndInvokesAfterRollback(t *testing.T) {
	db := migratedDB(t)
	users := repository.New[hookedUser](db)
	events := []string{}
	sentinel := errors.New("stop create")
	user := &hookedUser{Email: "fail@example.com", Events: &events, Fail: sentinel}
	if err := users.Create(t.Context(), user); !errors.Is(err, sentinel) {
		t.Fatalf("expected lifecycle error, got %v", err)
	}
	assertEvents(t, events, []string{"before_validate", "validate", "after_validate", "before_save", "before_create", "after_rollback"})
	if _, err := users.WithDeleted().Find(t.Context(), user.ID); !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		t.Fatalf("failed create persisted: %v", err)
	}
}

func TestNestedTransactionErrorMarksOuterRollbackOnly(t *testing.T) {
	db := migratedDB(t)
	users := repository.New[hookedUser](db)
	events := []string{}
	user := &hookedUser{Email: "nested@example.com", Events: &events}
	sentinel := errors.New("nested failure")
	err := users.Transaction(t.Context(), func(ctx context.Context, txUsers *repository.Repository[hookedUser]) error {
		if err := txUsers.Create(ctx, user); err != nil {
			return err
		}
		_ = db.Transaction(ctx, func(context.Context) error { return sentinel })
		return nil
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected nested rollback cause, got %v", err)
	}
	if !contains(events, "after_rollback") || contains(events, "after_commit") {
		t.Fatalf("terminal hook events = %v", events)
	}
	if _, err := users.WithDeleted().Find(t.Context(), user.ID); !soroerrors.IsCode(err, soroerrors.CodeNotFound) {
		t.Fatalf("rollback-only transaction persisted: %v", err)
	}
}

func migratedDB(t *testing.T) *database.DB {
	t.Helper()
	db := testdb.Open(t)
	if err := migrate.New(db).Apply(t.Context(), basic.Migrations); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return db
}

func assertEvents(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func causeOf(err error) error {
	var frameworkError *soroerrors.Error
	if errors.As(err, &frameworkError) {
		return errors.Unwrap(frameworkError)
	}
	return nil
}

type hookedUser struct {
	bun.BaseModel `bun:"table:users,alias:user"`
	model.Base
	Email  string `bun:"email,notnull" validate:"required,email"`
	Active bool   `bun:"active,notnull"`

	Events   *[]string `bun:"-"`
	Fail     error     `bun:"-"`
	OldEmail string    `bun:"-"`
	NewEmail string    `bun:"-"`
}

type principalID uuid.UUID

func (principal principalID) ID() uuid.UUID { return uuid.UUID(principal) }

func eventMetadata(events *[]string) repository.OperationOption {
	return repository.WithLifecycleMetadata(map[string]any{"events": events})
}

func (user *hookedUser) event(name string, lifecycleContext *lifecycle.Context) {
	events := user.Events
	if events == nil && lifecycleContext != nil {
		events, _ = lifecycleContext.Metadata["events"].(*[]string)
	}
	if events != nil {
		*events = append(*events, name)
	}
}

func (user *hookedUser) Validate(context.Context) error {
	user.event("validate", nil)
	return nil
}
func (user *hookedUser) BeforeValidate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_validate", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterValidate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_validate", lifecycleContext)
	return nil
}
func (user *hookedUser) BeforeSave(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_save", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterSave(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_save", lifecycleContext)
	return nil
}
func (user *hookedUser) BeforeCreate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_create", lifecycleContext)
	return user.Fail
}
func (user *hookedUser) AfterCreate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_create", lifecycleContext)
	return nil
}
func (user *hookedUser) BeforeUpdate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_update", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterUpdate(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_update", lifecycleContext)
	oldValue, newValue, ok := lifecycleContext.Changes.Values("Email")
	if ok {
		user.OldEmail, _ = oldValue.(string)
		user.NewEmail, _ = newValue.(string)
	}
	return nil
}
func (user *hookedUser) BeforeDelete(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_delete", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterDelete(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_delete", lifecycleContext)
	return nil
}
func (user *hookedUser) BeforeRestore(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_restore", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterRestore(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_restore", lifecycleContext)
	return nil
}
func (user *hookedUser) BeforeForceDelete(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("before_force_delete", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterForceDelete(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_force_delete", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterCommit(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_commit", lifecycleContext)
	return nil
}
func (user *hookedUser) AfterRollback(_ context.Context, lifecycleContext *lifecycle.Context) error {
	user.event("after_rollback", lifecycleContext)
	return nil
}
