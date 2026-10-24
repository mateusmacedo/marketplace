//go:build integration

package appkit_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/provider"
)

func TestTheDatabaseHoldsExactlyTheTablesOfThisContext(t *testing.T) {
	got := pg.Tables(t, appkit.OpenPool(t))

	want := []string{"inbox", "outbox", "quarantine", "stores"}
	if !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
}

func TestTheRuntimeRoleWritesStoresButNeverRemovesThem(t *testing.T) {
	pool := pg.OpenRuntimePool(t, appkit.PoolOptions)

	if _, err := pool.Exec(context.Background(), `INSERT INTO stores (tenant_id, store_id, status, snapshot) VALUES ('t-1', 'loja-1', 'active', '{"name":"Loja 1"}')`); err != nil {
		t.Fatalf("INSERT INTO stores by the runtime role = %v, want nil", err)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE stores SET version = version"); err != nil {
		t.Fatalf("UPDATE stores by the runtime role = %v, want nil", err)
	}
	for _, statement := range []string{"DELETE FROM stores", "TRUNCATE stores", "DROP TABLE stores", "ALTER TABLE stores ADD COLUMN intruder bigint"} {
		pg.AssertDenied(t, pool, statement)
	}
}

func TestTheSchemaRefusesAStatusOutsideTheLifecycle(t *testing.T) {
	pool := appkit.OpenPool(t)

	_, err := pool.Exec(context.Background(), `INSERT INTO stores (tenant_id, store_id, status, snapshot) VALUES ('t-1', 'loja-1', 'closed', '{"name":"Loja 1"}')`)

	if err == nil {
		t.Fatal("INSERT with status closed = nil, want stores_status_check to refuse it")
	}
}

func TestTheRuntimeRoleNeitherRewritesTheKernelTablesNorCreatesATable(t *testing.T) {
	pool := pg.OpenRuntimePool(t, appkit.PoolOptions)

	for _, statement := range []string{
		"UPDATE quarantine SET reason = reason",
		"DELETE FROM quarantine",
		"TRUNCATE outbox",
		"TRUNCATE inbox",
		"CREATE TABLE intruder (id bigint)",
	} {
		pg.AssertDenied(t, pool, statement)
	}
}

func TestTheSchemaHoldsTheUniquesTheProviderDeclares(t *testing.T) {
	if err := postgres.VerifyUniques(context.Background(), appkit.OpenPool(t), provider.Uniques...); err != nil {
		t.Fatalf("VerifyUniques(provider.Uniques) = %v, want nil", err)
	}
}
