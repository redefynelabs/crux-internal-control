package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/demo"
	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/store"
	"github.com/redefynelabs/erp-control-plane/internal/testutil"
)

// deleteServer is a server whose store the test can reach, with an admin session in `token`.
func deleteServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	cfg := &config.Config{TrustProxy: true,
		Demo: config.Demo{TTL: time.Hour, MaxLive: 10, MaxPerEmail: 5, MaxPerIP: 10, Domain: "demo.example.com", Scheme: "https"}}
	st := testutil.NewStore(t)
	token = adminSession(t, st)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	mailer := mail.New(st, "test-secret", log)
	return New(cfg, st, demo.New(cfg.Demo, st, okDBs{}, okStacks{}, log), mailer, sales.New(st, mailer, log), nil, log), st
}

func leadID(t *testing.T, st *store.Store, email string) string {
	t.Helper()
	l, err := st.LeadByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("lead %s: %v", email, err)
	}
	return l.ID
}

func TestDeletingALeadRemovesItAndItsRunningDemo(t *testing.T) {
	h, st := deleteServer(t)
	if w := do(h, "POST", "/v1/demo-requests", good); w.Code != 201 {
		t.Fatal(w.Body)
	}
	id := leadID(t, st, "asha@rao.example")
	if live, _ := st.CountLive(context.Background()); live != 1 {
		t.Fatalf("want one live demo, got %d", live)
	}

	if w := do(h, "DELETE", "/v1/leads/"+id, "", bearer()...); w.Code != 204 {
		t.Fatalf("want 204, got %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/v1/leads/"+id, "", bearer()...); w.Code != 404 {
		t.Errorf("the lead must be gone, got %d", w.Code)
	}
	if live, _ := st.CountLive(context.Background()); live != 0 {
		t.Errorf("its demo must have been ended, %d live", live)
	}
	if ds, _ := st.ListDemos(context.Background(), "", 50); len(ds) != 0 {
		t.Errorf("its demo rows must be gone, got %d", len(ds))
	}
	if insts, _ := st.ListInstallations(context.Background(), "demo"); len(insts) != 0 {
		t.Errorf("its demo installation must be gone, got %d", len(insts))
	}
	if au, _ := st.ListAudit(context.Background(), "", 10); len(au) == 0 || au[0].Action != "lead.deleted" {
		t.Errorf("the deletion must be in the audit log, got %+v", au)
	}
	if w := do(h, "DELETE", "/v1/leads/"+id, "", bearer()...); w.Code != 404 {
		t.Errorf("deleting twice must be 404, got %d", w.Code)
	}
}

func TestALeadWithASaleCannotBeDeleted(t *testing.T) {
	h, st := deleteServer(t)
	ctx := context.Background()
	l, _, err := st.UpsertLead(ctx, store.LeadInput{Name: "Buyer", Email: "buyer@x.example", Phone: "+91 98765 43210", Source: "website"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSale(ctx, store.SaleInput{LeadID: l.ID, Customer: "Buyer", Email: "buyer@x.example", Plan: "cloud", Billing: "monthly", Amount: 1000}); err != nil {
		t.Fatal(err)
	}
	w := do(h, "DELETE", "/v1/leads/"+l.ID, "", bearer()...)
	if w.Code != 409 || !strings.Contains(w.Body.String(), "has_records") {
		t.Fatalf("want 409 has_records, got %d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/v1/leads/"+l.ID, "", bearer()...); w.Code != 200 {
		t.Errorf("the lead must still exist, got %d", w.Code)
	}
}

func TestOnlyAnAdminCanDeleteALead(t *testing.T) {
	h, st := deleteServer(t)
	ctx := context.Background()
	l, _, err := st.UpsertLead(ctx, store.LeadInput{Name: "Someone", Email: "someone@x.example", Phone: "+91 98765 43210", Source: "website"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := st.CreateUser(ctx, "Seller", "seller@test.example", store.RoleSales, "x")
	if err != nil {
		t.Fatal(err)
	}
	tok, err := st.CreateSession(ctx, u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if w := do(h, "DELETE", "/v1/leads/"+l.ID, "", "Authorization", "Bearer "+tok); w.Code != 403 {
		t.Fatalf("a sales person must get 403, got %d %s", w.Code, w.Body)
	}
	if w := do(h, "DELETE", "/v1/leads/"+l.ID, ""); w.Code != 401 {
		t.Fatalf("no sign-in must get 401, got %d", w.Code)
	}
	if w := do(h, "GET", "/v1/leads/"+l.ID, "", bearer()...); w.Code != 200 {
		t.Errorf("the lead must still exist, got %d", w.Code)
	}
}
