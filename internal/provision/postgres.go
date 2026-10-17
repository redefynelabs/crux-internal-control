package provision

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
)

// Postgres clones databases from a template on one Postgres server.
type Postgres struct {
	adminURL string
	// Creating databases from the same template cannot overlap: Postgres refuses a second copy while the first runs.
	mu sync.Mutex
}

// NewPostgres returns a Databases that connects with adminURL, a role allowed to create databases (ending /postgres).
func NewPostgres(adminURL string) *Postgres { return &Postgres{adminURL: adminURL} }

func (p *Postgres) exec(ctx context.Context, sql string) error {
	conn, err := pgx.Connect(ctx, p.adminURL)
	if err != nil {
		return errors.New("provision: the demo database server is not reachable")
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, sql)
	return err
}

func quote(ident string) string { return `"` + ident + `"` }

// Clone implements Databases.
func (p *Postgres) Clone(ctx context.Context, template, name, owner string) error {
	for _, id := range []string{template, name, owner} {
		if !ValidIdent(id) {
			return fmt.Errorf("provision: %q is not a valid database or role name", id)
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exec(ctx, fmt.Sprintf("CREATE DATABASE %s TEMPLATE %s OWNER %s", quote(name), quote(template), quote(owner)))
}

// SetModules implements Databases.
func (p *Postgres) SetModules(ctx context.Context, name string, installed, disabled []string) error {
	if !ValidIdent(name) {
		return fmt.Errorf("provision: %q is not a valid database name", name)
	}
	cfg, err := pgx.ParseConfig(p.adminURL)
	if err != nil {
		return errors.New("provision: the demo database server is not reachable")
	}
	cfg.Database = name
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return errors.New("provision: the demo database is not reachable")
	}
	defer conn.Close(ctx)
	if installed == nil {
		installed = []string{}
	}
	if disabled == nil {
		disabled = []string{}
	}
	_, err = conn.Exec(ctx, `UPDATE organizations SET installed_modules = $1, disabled_features = $2, updated_at = now()`, installed, disabled)
	return err
}

// Drop implements Databases.
func (p *Postgres) Drop(ctx context.Context, name string) error {
	if !ValidIdent(name) {
		return fmt.Errorf("provision: %q is not a valid database name", name)
	}
	return p.exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", quote(name)))
}
