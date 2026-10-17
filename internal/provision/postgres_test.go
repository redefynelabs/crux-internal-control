package provision

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/redefynelabs/erp-control-plane/internal/testutil"
)

func exists(t *testing.T, admin, name string) bool {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var ok bool
	if err := conn.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestCloneCopiesTheTemplateAndDropRemovesIt(t *testing.T) {
	admin := testutil.AdminURL(t)
	ctx := context.Background()
	template := testutil.RandomName("cp_tpl_")
	clone := testutil.RandomName("cp_clone_")
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+template); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, _ := pgx.Connect(ctx, admin)
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, `DROP DATABASE IF EXISTS `+clone+` WITH (FORCE)`)
		_, _ = c.Exec(ctx, `DROP DATABASE IF EXISTS `+template+` WITH (FORCE)`)
	})
	tc, err := pgx.Connect(ctx, testutil.WithDatabase(t, admin, template))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tc.Exec(ctx, `CREATE TABLE marker (v int); INSERT INTO marker VALUES (42)`); err != nil {
		t.Fatal(err)
	}
	tc.Close(ctx)

	var owner string
	if err := conn.QueryRow(ctx, `SELECT current_user`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	p := NewPostgres(admin)
	if err := p.Clone(ctx, template, clone, owner); err != nil {
		t.Fatal(err)
	}
	cc, err := pgx.Connect(ctx, testutil.WithDatabase(t, admin, clone))
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := cc.QueryRow(ctx, `SELECT v FROM marker`).Scan(&v); err != nil || v != 42 {
		t.Fatalf("the clone should hold the template's data: %d %v", v, err)
	}
	// Drop works even while someone is connected.
	if err := p.Drop(ctx, clone); err != nil {
		t.Fatal(err)
	}
	cc.Close(ctx)
	if exists(t, admin, clone) {
		t.Fatal("the clone should be gone")
	}
	if err := p.Drop(ctx, clone); err != nil {
		t.Fatalf("dropping what is gone is not an error: %v", err)
	}
}

func TestNamesArePutIntoSQLSoTheyAreChecked(t *testing.T) {
	p := NewPostgres("postgres://unused")
	for _, bad := range []string{`x"; DROP DATABASE y; --`, "Upper", "", "a b"} {
		if err := p.Clone(context.Background(), "tpl", bad, "owner"); err == nil {
			t.Errorf("%q must be refused", bad)
		}
		if err := p.Drop(context.Background(), bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}
