// Package provision creates and removes the pieces of a demo instance: its database and its containers. Both are behind
// interfaces so the demo service can be tested without Postgres or Docker.
package provision

import (
	"context"
	"regexp"
	"time"
)

// Databases clones and drops the per-instance databases on the shared Postgres.
type Databases interface {
	// Clone creates name as a copy of template, owned by owner.
	Clone(ctx context.Context, template, name, owner string) error
	// Drop removes name, closing any connection to it. A database that does not exist is not an error.
	Drop(ctx context.Context, name string) error
	// SetModules sets which apps are installed, and which features are switched off, for the organization in the
	// database name, so a copy shows only what its visitor asked to see.
	SetModules(ctx context.Context, name string, installed, disabled []string) error
}

// StackSpec describes the containers of one instance.
type StackSpec struct {
	Slug        string
	ServerImage string
	WebImage    string
	// ServerEnv is passed to the server container through a private file, never on the command line.
	ServerEnv map[string]string
	// Host is the public host name the proxy routes to the web container.
	Host string
	// Memory and CPUs cap each container ("512m", "0.5").
	Memory string
	CPUs   string
}

// Stacks starts and stops instances.
type Stacks interface {
	Start(ctx context.Context, spec StackSpec) error
	// WaitReady returns when the instance answers, or an error when it does not within timeout.
	WaitReady(ctx context.Context, slug string, timeout time.Duration) error
	// Stop removes the instance's containers and network. Missing pieces are not an error.
	Stop(ctx context.Context, slug string) error
	// List returns the slugs that have containers now.
	List(ctx context.Context) ([]string, error)
}

var identPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// ValidIdent says whether s is safe to use as a database or role name. Names are chosen by the control plane, never by
// a visitor, and are still checked because they are put into SQL.
func ValidIdent(s string) bool { return identPattern.MatchString(s) }

var slugPattern = regexp.MustCompile(`^[a-z0-9]{6,20}$`)

// ValidSlug says whether s is safe to use in container, network, database and host names.
func ValidSlug(s string) bool { return slugPattern.MatchString(s) }
