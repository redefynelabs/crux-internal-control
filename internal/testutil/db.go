// Package testutil gives tests a scratch Postgres database. Set CP_TEST_DATABASE_URL to a role that may create
// databases (ending /postgres); without it the tests that need Postgres are skipped.
package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/redefynelabs/erp-control-plane/internal/store"
)

// AdminURL returns the admin connection string, or skips the test.
func AdminURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("CP_TEST_DATABASE_URL")
	if u == "" {
		t.Skip("CP_TEST_DATABASE_URL is not set")
	}
	return u
}

// WithDatabase returns adminURL with another database name.
func WithDatabase(t *testing.T, adminURL, name string) string {
	t.Helper()
	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// RandomName returns a lower-case database name starting with prefix.
func RandomName(prefix string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// ScratchDatabase creates an empty database and drops it when the test ends. It returns its URL.
func ScratchDatabase(t *testing.T) string {
	t.Helper()
	admin := AdminURL(t)
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	name := RandomName("cp_test_")
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(ctx, admin)
		if err != nil {
			return
		}
		defer c.Close(ctx)
		_, _ = c.Exec(ctx, `DROP DATABASE IF EXISTS `+name+` WITH (FORCE)`)
	})
	return WithDatabase(t, admin, name)
}

// NewStore opens a migrated store on a scratch database.
func NewStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), ScratchDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}
