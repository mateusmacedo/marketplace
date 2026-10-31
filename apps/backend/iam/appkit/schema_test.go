//go:build integration

package appkit_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func TestTheDatabaseHoldsExactlyTheTablesOfThisContext(t *testing.T) {
	got := pg.Tables(t, appkit.OpenPool(t))

	want := []string{"buyers", "inbox", "invitations", "memberships", "outbox", "quarantine", "roles"}
	if !slices.Equal(got, want) {
		t.Fatalf("tables = %v, want %v", got, want)
	}
}

var aggregateRows = map[string]string{
	"roles":       `INSERT INTO roles (tenant_id, role_id, status, snapshot) VALUES ('loja-1', 'expedicao', '%s', '{"name":"Expedição"}')`,
	"invitations": `INSERT INTO invitations (tenant_id, invitation_id, status, snapshot) VALUES ('loja-1', 'ana@loja.com', '%s', '{}')`,
	"memberships": `INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-1', 'kc-123', '%s', '{"roles":["expedicao"]}')`,
	"buyers":      `INSERT INTO buyers (tenant_id, buyer_id, status, snapshot) VALUES ('marketplace', 'kc-123', '%s', '{"name":"Bia"}')`,
}

var liveStatus = map[string]string{"roles": "active", "invitations": "pending", "memberships": "active", "buyers": "active"}

func TestTheRuntimeRoleWritesTheAggregatesButNeverRemovesThem(t *testing.T) {
	pool := pg.OpenRuntimePool(t, appkit.PoolOptions)

	for table, row := range aggregateRows {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(row, liveStatus[table])); err != nil {
			t.Fatalf("INSERT INTO %s by the runtime role = %v, want nil", table, err)
		}
		if _, err := pool.Exec(context.Background(), "UPDATE "+table+" SET version = version"); err != nil {
			t.Fatalf("UPDATE %s by the runtime role = %v, want nil", table, err)
		}
		for _, statement := range []string{"DELETE FROM ", "TRUNCATE ", "DROP TABLE "} {
			pg.AssertDenied(t, pool, statement+table)
		}
		pg.AssertDenied(t, pool, "ALTER TABLE "+table+" ADD COLUMN intruder bigint")
	}
}

func TestTheSchemaRefusesAStatusOutsideTheLifecycle(t *testing.T) {
	pool := appkit.OpenPool(t)

	for table, row := range aggregateRows {
		if _, err := pool.Exec(context.Background(), fmt.Sprintf(row, "deleted")); err == nil {
			t.Errorf("INSERT INTO %s with status deleted = nil, want %s_status_check to refuse it", table, table)
		}
	}
}

// The generated owner column is what the unique constraint reads: true only
// for an active membership holding owner, NULL for every other one.
func TestTheOwnerColumnMarksOnlyTheActiveOwner(t *testing.T) {
	pool := appkit.OpenPool(t)
	for _, row := range []string{
		`INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-1', 'kc-owner', 'active', '{"roles":["owner"]}')`,
		`INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-1', 'kc-old', 'removed', '{"roles":["owner"]}')`,
		`INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-1', 'kc-a', 'active', '{"roles":["expedicao"]}')`,
		`INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-1', 'kc-b', 'active', '{"roles":["catalogo"]}')`,
	} {
		if _, err := pool.Exec(context.Background(), row); err != nil {
			t.Fatalf("%s = %v, want nil", row, err)
		}
	}

	var owners int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM memberships WHERE owner").Scan(&owners); err != nil || owners != 1 {
		t.Fatalf("owners = %d, %v; want only kc-owner", owners, err)
	}
	_, err := pool.Exec(context.Background(), `INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-1', 'kc-c', 'active', '{"roles":["catalogo","owner"]}')`)
	if err == nil {
		t.Fatal("a second active owner in loja-1 = nil, want memberships_tenant_id_owner_key to refuse it")
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO memberships (tenant_id, membership_id, status, snapshot) VALUES ('loja-2', 'kc-owner', 'active', '{"roles":["owner"]}')`); err != nil {
		t.Fatalf("the owner of loja-2 = %v, want nil: the constraint starts at tenant_id", err)
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
