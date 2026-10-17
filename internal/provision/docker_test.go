package provision

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	calls   [][]string
	envSeen string // the contents of the --env-file while the command ran
	failAt  string // fail the command that starts with this text
	outputs map[string]string
}

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	f.calls = append(f.calls, args)
	line := strings.Join(args, " ")
	for i, a := range args {
		if a == "--env-file" {
			b, _ := os.ReadFile(args[i+1])
			f.envSeen = string(b)
		}
	}
	if f.failAt != "" && strings.HasPrefix(line, f.failAt) {
		return "", errors.New("docker failed")
	}
	for prefix, out := range f.outputs {
		if strings.HasPrefix(line, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func (f *fakeRunner) has(prefix string) bool {
	for _, c := range f.calls {
		if strings.HasPrefix(strings.Join(c, " "), prefix) {
			return true
		}
	}
	return false
}

func spec() StackSpec {
	return StackSpec{Slug: "abcd2345", ServerImage: "erp-server:v1", WebImage: "erp-web:v1", Host: "abcd2345.demo.example.com", Memory: "384m", CPUs: "0.5",
		ServerEnv: map[string]string{"DATABASE_URL": "postgres://u:supersecret@pg/db", "JWT_SECRET": "j"}}
}

func newDocker(r Runner) *Docker {
	return NewDocker(r, DockerConfig{DataNetwork: "erp-data", ProxyNetwork: "erp-proxy", Entrypoint: "websecure", CertResolver: "le"})
}

func TestStartBuildsAPrivateNetworkAndRoutesOnlyTheWebContainer(t *testing.T) {
	r := &fakeRunner{}
	if err := newDocker(r).Start(context.Background(), spec()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"network create --label cp.demo=abcd2345 cp-abcd2345",
		"network connect erp-data cp-abcd2345-server",
		"network connect erp-proxy cp-abcd2345-web",
	} {
		if !r.has(want) {
			t.Errorf("missing: %s", want)
		}
	}
	all := ""
	for _, c := range r.calls {
		all += strings.Join(c, " ") + "\n"
	}
	if !strings.Contains(all, "--network-alias server") || !strings.Contains(all, "Host(`abcd2345.demo.example.com`)") || !strings.Contains(all, "loadbalancer.server.port=3000") {
		t.Errorf("the web app must reach the server as 'server' and be routed by host:\n%s", all)
	}
	if strings.Count(all, "traefik.enable=true") != 1 {
		t.Errorf("only the web container is exposed:\n%s", all)
	}
	if !strings.Contains(all, "--memory 384m") || !strings.Contains(all, "--read-only") {
		t.Errorf("limits and a read-only filesystem:\n%s", all)
	}
	if !strings.Contains(all, "--cap-drop ALL") || !strings.Contains(all, "no-new-privileges") {
		t.Errorf("no capabilities and no new privileges:\n%s", all)
	}
}

func TestSecretsGoThroughAPrivateFileNotTheCommandLine(t *testing.T) {
	r := &fakeRunner{}
	d := newDocker(r)
	d.tempDir = t.TempDir()
	if err := d.Start(context.Background(), spec()); err != nil {
		t.Fatal(err)
	}
	for _, c := range r.calls {
		if strings.Contains(strings.Join(c, " "), "supersecret") {
			t.Fatalf("a secret reached the command line: %v", c)
		}
	}
	if !strings.Contains(r.envSeen, "DATABASE_URL=postgres://u:supersecret@pg/db") {
		t.Fatalf("the env file should hold the settings, got %q", r.envSeen)
	}
	if left, _ := os.ReadDir(d.tempDir); len(left) != 0 {
		t.Fatalf("the env file must be deleted after the start: %v", left)
	}
}

func TestAFailedStartRemovesWhatItMade(t *testing.T) {
	r := &fakeRunner{failAt: "network connect erp-proxy"}
	if err := newDocker(r).Start(context.Background(), spec()); err == nil {
		t.Fatal("the start should fail")
	}
	for _, want := range []string{"rm -f cp-abcd2345-web", "rm -f cp-abcd2345-server", "network rm cp-abcd2345"} {
		if !r.has(want) {
			t.Errorf("cleanup missing: %s", want)
		}
	}
}

func TestRefusesAnUnsafeSlugAndAValueWithALineBreak(t *testing.T) {
	d := newDocker(&fakeRunner{})
	bad := spec()
	bad.Slug = "a;rm -rf"
	if err := d.Start(context.Background(), bad); err == nil {
		t.Fatal("an unsafe slug must be refused")
	}
	bad = spec()
	bad.ServerEnv = map[string]string{"X": "a\nINJECTED=1"}
	if err := d.Start(context.Background(), bad); err == nil {
		t.Fatal("a line break could add a setting; refuse it")
	}
}

func TestStopToleratesPiecesThatAreAlreadyGone(t *testing.T) {
	r := &fakeRunner{}
	r.failAt = "never"
	d := newDocker(&goneRunner{})
	if err := d.Stop(context.Background(), "abcd2345"); err != nil {
		t.Fatalf("missing containers and networks are not errors: %v", err)
	}
	_ = r
}

type goneRunner struct{}

func (goneRunner) Run(_ context.Context, args ...string) (string, error) {
	if args[0] == "rm" {
		return "", errors.New("docker rm: Error response from daemon: No such container: x")
	}
	return "", errors.New("docker network: Error response from daemon: network cp-abcd2345 not found")
}

func TestListReturnsEachSlugOnce(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{"ps": "abcd2345\nabcd2345\nzzzz9999\n\n"}}
	slugs, err := newDocker(r).List(context.Background())
	if err != nil || len(slugs) != 2 {
		t.Fatalf("got %v %v", slugs, err)
	}
}

func TestWaitReadyNeedsBothContainersToAnswer(t *testing.T) {
	r := &fakeRunner{}
	if err := newDocker(r).WaitReady(context.Background(), "abcd2345", time.Second); err != nil {
		t.Fatal(err)
	}
	if !r.has("exec cp-abcd2345-server /app/server healthcheck") || !r.has("exec cp-abcd2345-web wget") {
		t.Fatalf("both checks must run: %v", r.calls)
	}
	failing := &fakeRunner{failAt: "exec cp-abcd2345-web"}
	if err := newDocker(failing).WaitReady(context.Background(), "abcd2345", 10*time.Millisecond); err == nil {
		t.Fatal("a web app that never answers is not ready")
	}
}
