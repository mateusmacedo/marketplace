package ports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/ports"
)

// readerFunc realizes StoresByStatusReader over a function, so the contract is
// exercised without reaching the provider block.
type readerFunc func(context.Context, domain.StoreStatus) ([]domain.StoreSnapshot, error)

func (f readerFunc) LoadByStatus(ctx context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error) {
	return f(ctx, status)
}

func TestLoadByStatusAnswersWithEveryStoreInThatStatus(t *testing.T) {
	active := []domain.StoreSnapshot{
		{Ref: "loja-a", Name: "Loja A", Status: domain.Active, RegisteredAt: 1, StatusChangedAt: 1},
		{Ref: "loja-b", Name: "Loja B", Status: domain.Active, RegisteredAt: 2, StatusChangedAt: 2},
	}
	var asked domain.StoreStatus
	var reader ports.StoresByStatusReader = readerFunc(func(_ context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error) {
		asked = status
		return active, nil
	})

	got, err := reader.LoadByStatus(context.Background(), domain.Active)

	if err != nil {
		t.Fatalf("LoadByStatus returned %v, want no error", err)
	}
	if asked != domain.Active {
		t.Fatalf("the reader was keyed on %v, want Active", asked)
	}
	if len(got) != len(active) || got[0].Ref != "loja-a" || got[1].Ref != "loja-b" {
		t.Fatalf("LoadByStatus returned %+v, want %+v: the query answers with a collection", got, active)
	}
}

func TestLoadByStatusAnswersWithNoStoreWithoutFailing(t *testing.T) {
	var reader ports.StoresByStatusReader = readerFunc(func(context.Context, domain.StoreStatus) ([]domain.StoreSnapshot, error) {
		return nil, nil
	})

	got, err := reader.LoadByStatus(context.Background(), domain.Suspended)

	if err != nil || len(got) != 0 {
		t.Fatalf("LoadByStatus = %+v, %v; want an empty answer, not a failure", got, err)
	}
}

func TestLoadByStatusKeepsTheCallerContext(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "marker")
	var seen context.Context
	var reader ports.StoresByStatusReader = readerFunc(func(ctx context.Context, _ domain.StoreStatus) ([]domain.StoreSnapshot, error) {
		seen = ctx
		return nil, nil
	})

	if _, err := reader.LoadByStatus(parent, domain.Active); err != nil {
		t.Fatalf("LoadByStatus returned %v, want no error", err)
	}

	if seen.Value(key{}) != "marker" {
		t.Fatal("LoadByStatus dropped the caller context; deadline and tenant must reach the provider")
	}
}

func TestLoadByStatusSurfacesTheProviderErrorUntranslated(t *testing.T) {
	cause := errors.New("connection reset by peer")
	var reader ports.StoresByStatusReader = readerFunc(func(context.Context, domain.StoreStatus) ([]domain.StoreSnapshot, error) {
		return nil, cause
	})

	got, err := reader.LoadByStatus(context.Background(), domain.Active)

	if !errors.Is(err, cause) || got != nil {
		t.Fatalf("LoadByStatus = %+v, %v; want no stores and an error carrying %v", got, err, cause)
	}
}
