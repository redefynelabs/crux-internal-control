// Command controlplane is the vendor's back office: it takes demo requests from the marketing site, starts and removes
// demo instances, and keeps the leads.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/redefynelabs/erp-control-plane/internal/api"
	"github.com/redefynelabs/erp-control-plane/internal/config"
	"github.com/redefynelabs/erp-control-plane/internal/demo"
	"github.com/redefynelabs/erp-control-plane/internal/entitle"
	"github.com/redefynelabs/erp-control-plane/internal/mail"
	"github.com/redefynelabs/erp-control-plane/internal/notify"
	"github.com/redefynelabs/erp-control-plane/internal/provision"
	"github.com/redefynelabs/erp-control-plane/internal/sales"
	"github.com/redefynelabs/erp-control-plane/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 3 && os.Args[1] == "reset-password" {
		return resetPassword(os.Args[2])
	}
	if len(os.Args) >= 2 && (os.Args[1] == "keygen" || os.Args[1] == "public-key") {
		return keyCommand(os.Args[1], os.Args[2:])
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	d := cfg.Demo
	stacks := provision.NewDocker(provision.ExecRunner{}, provision.DockerConfig{
		DataNetwork: d.DataNetwork, ProxyNetwork: d.ProxyNetwork, Entrypoint: d.Entrypoint, CertResolver: d.CertResolver,
	})
	demos := demo.New(d, st, provision.NewPostgres(d.PGAdminURL), stacks, log)
	if cfg.NotifyWebhookURL != "" {
		demos.WithNotifier(notify.Webhook{URL: cfg.NotifyWebhookURL, Log: log})
	}
	var signer *entitle.Signer
	if cfg.EntitlementKeyFile != "" {
		// A key file others can read is refused here, so the server does not start with a leaked signing key.
		signer, err = entitle.LoadKey(cfg.EntitlementKeyFile)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			// An upgrade that has not made the key yet still starts; issuing says why it cannot.
			log.Warn("the entitlement key file does not exist yet: run controlplane keygen", "file", cfg.EntitlementKeyFile)
			signer = nil
		case err != nil:
			return err
		default:
			log.Info("entitlements can be issued", "public_key", signer.PublicKey())
		}
	} else {
		log.Warn("CP_ENTITLEMENT_KEY_FILE is not set: entitlements cannot be issued")
	}
	issuer := entitle.NewIssuer(st, signer)
	demos.WithEntitlements(issuer)
	mailer := mail.New(st, cfg.SecretsKey, log)
	team := sales.New(st, mailer, log)
	demos.WithIntake(team)
	go demos.RunReaper(ctx, time.Minute)
	go mailer.RunWorker(ctx, 15*time.Second)
	if err := bootstrapAdmin(ctx, st, log); err != nil {
		return err
	}

	var captcha api.Captcha
	if cfg.TurnstileSecret != "" {
		captcha = api.Turnstile{Secret: cfg.TurnstileSecret}
	}
	srv := &http.Server{
		Addr: cfg.Listen, Handler: api.New(cfg, st, demos, mailer, team, captcha, log, api.WithEntitlements(issuer)),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 4 * time.Minute, IdleTimeout: time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Info("control plane listening", "addr", cfg.Listen)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// bootstrapAdmin creates the first admin on the first start, from CP_ADMIN_EMAIL and CP_ADMIN_PASSWORD. Once anyone
// exists it does nothing: the team is managed from the admin app, and a forgotten password is reset with
// `controlplane reset-password`.
func bootstrapAdmin(ctx context.Context, st *store.Store, log *slog.Logger) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	email, pass := os.Getenv("CP_ADMIN_EMAIL"), os.Getenv("CP_ADMIN_PASSWORD")
	if email == "" || pass == "" {
		return errors.New("there is nobody to sign in yet: set CP_ADMIN_EMAIL and CP_ADMIN_PASSWORD for the first start")
	}
	if len(pass) < 10 {
		return errors.New("CP_ADMIN_PASSWORD must be at least 10 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	name := os.Getenv("CP_ADMIN_NAME")
	if name == "" {
		name = "Admin"
	}
	if _, err := st.CreateUser(ctx, name, email, store.RoleAdmin, string(hash)); err != nil {
		return err
	}
	log.Info("first admin created", "email", email)
	return nil
}

// resetPassword is `controlplane reset-password <email>`: sets a new password (from CP_NEW_PASSWORD, never an argument,
// so it stays out of shell history and process lists), reactivates the person and ends their sessions. It is the way back
// in when the last admin has forgotten their password.
func resetPassword(email string) error {
	pass := os.Getenv("CP_NEW_PASSWORD")
	if len(pass) < 10 {
		return errors.New("set CP_NEW_PASSWORD (at least 10 characters)")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	u, _, err := st.Credentials(ctx, email)
	if err != nil {
		return errors.New("nobody has that email")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := st.SetPassword(ctx, u.ID, string(hash)); err != nil {
		return err
	}
	active := true
	if _, err := st.UpdateUser(ctx, u.ID, store.UserPatch{Active: &active}); err != nil {
		return err
	}
	_ = st.EndSessions(ctx, u.ID)
	fmt.Println("password changed for", u.Email)
	return nil
}

// keyCommand is `controlplane keygen [file]` and `controlplane public-key [file]`. The file defaults to
// CP_ENTITLEMENT_KEY_FILE. keygen makes the signing key (mode 0600, never replacing one) and prints the public key to set
// as ENTITLEMENT_PUBLIC_KEY on every installation; public-key shows it again.
func keyCommand(name string, args []string) error {
	path := os.Getenv("CP_ENTITLEMENT_KEY_FILE")
	if len(args) > 0 {
		path = args[0]
	}
	if path == "" {
		return fmt.Errorf("give the key file: controlplane %s <file>, or set CP_ENTITLEMENT_KEY_FILE", name)
	}
	var (
		signer *entitle.Signer
		err    error
	)
	if name == "keygen" {
		if signer, err = entitle.GenerateKey(path); err != nil {
			return err
		}
		fmt.Println("key written to", path, "(keep it out of the repository and of backups that leave the server)")
	} else if signer, err = entitle.LoadKey(path); err != nil {
		return err
	}
	fmt.Println("ENTITLEMENT_PUBLIC_KEY=" + signer.PublicKey())
	return nil
}
