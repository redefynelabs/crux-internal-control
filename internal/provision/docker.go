package provision

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Runner runs the docker command line. The real one is ExecRunner; tests use a fake.
type Runner interface {
	Run(ctx context.Context, args ...string) (string, error)
}

// ExecRunner runs the docker binary.
type ExecRunner struct{ Binary string }

// Run implements Runner. The error carries docker's own message and never the arguments, which can hold file paths.
func (r ExecRunner) Run(ctx context.Context, args ...string) (string, error) {
	bin := r.Binary
	if bin == "" {
		bin = "docker"
	}
	var out, errb bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("docker %s: %s", args[0], strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// DockerConfig is where instances are attached.
type DockerConfig struct {
	// DataNetwork is the network Postgres and Redis are on. The server container joins it.
	DataNetwork string
	// ProxyNetwork is the network the reverse proxy (Traefik) is on. The web container joins it.
	ProxyNetwork string
	// Entrypoint and CertResolver are Traefik's, for the HTTPS router.
	Entrypoint   string
	CertResolver string
}

// Docker runs each instance as two containers (server, web) on a private network of their own.
type Docker struct {
	run Runner
	cfg DockerConfig
	// tempDir is where the private env files are written; "" means the OS default.
	tempDir string
}

// NewDocker returns a Stacks that uses the docker command line.
func NewDocker(r Runner, cfg DockerConfig) *Docker { return &Docker{run: r, cfg: cfg} }

func names(slug string) (network, server, web string) {
	return "cp-" + slug, "cp-" + slug + "-server", "cp-" + slug + "-web"
}

// Start implements Stacks. On any failure it removes what it created, so a failed start leaves nothing behind.
func (d *Docker) Start(ctx context.Context, s StackSpec) (err error) {
	if !ValidSlug(s.Slug) {
		return fmt.Errorf("provision: %q is not a valid slug", s.Slug)
	}
	network, server, web := names(s.Slug)
	defer func() {
		if err != nil {
			_ = d.Stop(context.WithoutCancel(ctx), s.Slug)
		}
	}()

	envFile, err := d.writeEnv(s.ServerEnv)
	if err != nil {
		return err
	}
	defer os.Remove(envFile)

	label := "cp.demo=" + s.Slug
	if _, err = d.run.Run(ctx, "network", "create", "--label", label, network); err != nil {
		return err
	}
	// Visitors run nothing of their own, but the demo shares a host with the control plane, so each container gets no
	// extra privileges and no capabilities, and cannot gain any later.
	limits := []string{"--memory", s.Memory, "--cpus", s.CPUs, "--pids-limit", "512", "--cap-drop", "ALL", "--security-opt", "no-new-privileges"}
	// Created, attached to the data network, then started: the server pings Postgres once at startup and exits if it
	// cannot be resolved yet.
	serverArgs := append([]string{"create", "--name", server, "--network", network, "--network-alias", "server",
		"--label", label, "--env-file", envFile, "--restart", "no", "--read-only", "--tmpfs", "/tmp"}, limits...)
	if _, err = d.run.Run(ctx, append(serverArgs, s.ServerImage)...); err != nil {
		return err
	}
	if _, err = d.run.Run(ctx, "network", "connect", d.cfg.DataNetwork, server); err != nil {
		return err
	}
	if _, err = d.run.Run(ctx, "start", server); err != nil {
		return err
	}
	router := "cp-" + s.Slug
	webArgs := append([]string{"run", "-d", "--name", web, "--network", network, "--label", label,
		"--label", "traefik.enable=true",
		"--label", "traefik.docker.network=" + d.cfg.ProxyNetwork,
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s`)", router, s.Host),
		"--label", fmt.Sprintf("traefik.http.routers.%s.entrypoints=%s", router, d.cfg.Entrypoint),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=3000", router),
		"--restart", "no"}, limits...)
	if d.cfg.CertResolver != "" {
		// No resolver means plain HTTP, for trying the whole flow on a laptop.
		webArgs = append(webArgs, "--label", fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=%s", router, d.cfg.CertResolver))
	}
	if _, err = d.run.Run(ctx, append(webArgs, s.WebImage)...); err != nil {
		return err
	}
	_, err = d.run.Run(ctx, "network", "connect", d.cfg.ProxyNetwork, web)
	return err
}

func (d *Docker) writeEnv(env map[string]string) (string, error) {
	f, err := os.CreateTemp(d.tempDir, "cp-env-*")
	if err != nil {
		return "", err
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.ContainsAny(env[k], "\r\n") {
			f.Close()
			os.Remove(f.Name())
			return "", fmt.Errorf("provision: value of %s has a line break", k)
		}
		fmt.Fprintf(f, "%s=%s\n", k, env[k])
	}
	return f.Name(), f.Close()
}

// WaitReady implements Stacks: the server must pass its own health check and the web app must serve its login page.
func (d *Docker) WaitReady(ctx context.Context, slug string, timeout time.Duration) error {
	_, server, web := names(slug)
	deadline := time.Now().Add(timeout)
	for {
		_, errS := d.run.Run(ctx, "exec", server, "/app/server", "healthcheck")
		var errW error
		if errS == nil {
			_, errW = d.run.Run(ctx, "exec", web, "wget", "-qO-", "http://127.0.0.1:3000/login")
		}
		if errS == nil && errW == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("provision: the instance did not become ready in time")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

// Stop implements Stacks.
func (d *Docker) Stop(ctx context.Context, slug string) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("provision: %q is not a valid slug", slug)
	}
	network, server, web := names(slug)
	var errs []error
	// -f stops and removes in one step; a container that is already gone answers an error that is not worth keeping.
	for _, c := range []string{web, server} {
		if out, err := d.run.Run(ctx, "rm", "-f", c); err != nil && !strings.Contains(err.Error()+out, "No such container") {
			errs = append(errs, err)
		}
	}
	if out, err := d.run.Run(ctx, "network", "rm", network); err != nil && !strings.Contains(err.Error()+out, "not found") {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// List implements Stacks.
func (d *Docker) List(ctx context.Context) ([]string, error) {
	out, err := d.run.Run(ctx, "ps", "-a", "--filter", "label=cp.demo", "--format", `{{.Label "cp.demo"}}`)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var slugs []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" && !seen[s] {
			seen[s] = true
			slugs = append(slugs, s)
		}
	}
	return slugs, nil
}
