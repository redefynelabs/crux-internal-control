// Package config reads the control plane's settings from the environment. Nothing secret is printed on an error.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every setting.
type Config struct {
	Listen      string
	DatabaseURL string
	// AllowedOrigins may call the public demo endpoint from a browser (the marketing site).
	AllowedOrigins []string
	// TrustProxy reads the visitor's address from X-Forwarded-For (set when a reverse proxy is in front).
	TrustProxy bool
	// NotifyWebhookURL receives a one-line message for each new lead, contact message and demo (Slack-style JSON).
	// Empty: no notifications.
	NotifyWebhookURL string
	// AdminURL is the public address of the admin app; invitation emails link to it. Empty: invites are shown as links
	// on screen only.
	AdminURL string
	// SecretsKey encrypts the SMTP password stored in the database. Required, at least 32 characters.
	SecretsKey string
	// TurnstileSecret enables Cloudflare Turnstile checks on demo requests. Empty: no captcha.
	TurnstileSecret string

	// EntitlementKeyFile (CP_ENTITLEMENT_KEY_FILE) holds the Ed25519 private key entitlements are signed with, outside the
	// repository and database dumps, readable only by its owner. Empty: nothing can be issued.
	EntitlementKeyFile string

	Install Install

	Demo Demo
}

// Install is what a customer's one-line installer is given. All optional: without Registry nothing can be installed from here.
type Install struct {
	// PublicURL is this service's public address (CP_PUBLIC_URL, for example https://api.vendor.example). The installer
	// script is served with it filled in, and the customer page shows the command with it.
	PublicURL string
	// Registry is where the images are (CP_INSTALL_REGISTRY, for example ghcr.io/vendor). User and Token (CP_INSTALL_REGISTRY_USER,
	// CP_INSTALL_REGISTRY_TOKEN) may only read them; empty when the images are public. They are handed to a customer
	// who holds a valid activation code, and to nobody else.
	Registry string
	User     string
	Token    string
}

// Demo is everything about demo instances.
type Demo struct {
	TTL time.Duration
	// MaxLive is how many demos may run at once; MaxPerEmail and MaxPerIP limit requests in 24 hours.
	MaxLive     int
	MaxPerEmail int
	MaxPerIP    int
	// MaxQueue is how many visitors may wait for a free slot (0: nobody waits, they are told to try later); a waiting
	// request is dropped, with an apology, after QueueWait.
	MaxQueue  int
	QueueWait time.Duration

	// Domain is the parent of demo addresses: <slug>.<Domain>.
	Domain string
	Scheme string

	ServerImage string
	WebImage    string
	Memory      string
	CPUs        string

	// PGAdminURL creates and drops databases; PGHost, PGUser and PGPassword are how the demo server reaches them.
	PGAdminURL  string
	PGHost      string
	PGUser      string
	PGPassword  string
	Template    string
	RedisURL    string
	DataNetwork string

	ProxyNetwork string
	Entrypoint   string
	CertResolver string

	// EntitlementTemplate is the industry template named in the entitlement each demo is given.
	EntitlementTemplate string

	// AIProvider, AIModel and AIKey let visitors try Mei in a demo with our own AI key (passed to the demo server as
	// DEMO_AI_*). AIMaxQuestions is how many questions one demo may ask. Empty key: Mei is not usable in demos.
	AIProvider     string
	AIModel        string
	AIKey          string
	AIMaxQuestions int
}

// Load reads the environment.
func Load() (*Config, error) { return load(os.LookupEnv) }

func load(lookup func(string) (string, bool)) (*Config, error) {
	var problems []string
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	need := func(key string) string {
		v := get(key, "")
		if v == "" {
			problems = append(problems, key+": required")
		}
		return v
	}
	num := func(key string, def int) int {
		v := get(key, "")
		if v == "" {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			problems = append(problems, key+": must be a positive number")
			return def
		}
		return n
	}

	c := &Config{
		Listen:             get("CP_LISTEN", ":8090"),
		DatabaseURL:        need("CP_DATABASE_URL"),
		TurnstileSecret:    get("CP_TURNSTILE_SECRET", ""),
		AdminURL:           strings.TrimRight(get("CP_ADMIN_URL", ""), "/"),
		SecretsKey:         need("CP_SECRETS_KEY"),
		TrustProxy:         get("CP_TRUST_PROXY", "") == "true",
		NotifyWebhookURL:   get("CP_NOTIFY_WEBHOOK_URL", ""),
		EntitlementKeyFile: get("CP_ENTITLEMENT_KEY_FILE", ""),
	}
	if len(c.SecretsKey) > 0 && len(c.SecretsKey) < 32 {
		problems = append(problems, "CP_SECRETS_KEY: must be at least 32 characters")
	}
	for _, o := range strings.Split(get("CP_ALLOWED_ORIGINS", ""), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			c.AllowedOrigins = append(c.AllowedOrigins, o)
		}
	}
	if u := c.NotifyWebhookURL; u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		problems = append(problems, "CP_NOTIFY_WEBHOOK_URL: must be an http(s) URL")
	}
	ttl, err := time.ParseDuration(get("CP_DEMO_TTL", "1h"))
	if err != nil || ttl < time.Minute || ttl > 24*time.Hour {
		problems = append(problems, "CP_DEMO_TTL: must be a duration between 1m and 24h, such as 1h")
		ttl = time.Hour
	}
	queueMax := 30
	if v := get("CP_DEMO_QUEUE_MAX", ""); v != "" {
		if n, err := strconv.Atoi(v); err != nil || n < 0 {
			problems = append(problems, "CP_DEMO_QUEUE_MAX: must be 0 or a positive number")
		} else {
			queueMax = n
		}
	}
	queueWait, err := time.ParseDuration(get("CP_DEMO_QUEUE_WAIT", "24h"))
	if err != nil || queueWait < time.Minute || queueWait > 7*24*time.Hour {
		problems = append(problems, "CP_DEMO_QUEUE_WAIT: must be a duration between 1m and 168h, such as 24h")
		queueWait = 24 * time.Hour
	}
	c.Demo = Demo{
		MaxQueue: queueMax, QueueWait: queueWait,
		TTL: ttl, MaxLive: num("CP_MAX_DEMOS", 20), MaxPerEmail: num("CP_DEMO_PER_EMAIL_24H", 2), MaxPerIP: num("CP_DEMO_PER_IP_24H", 5),
		Domain: need("CP_DEMO_DOMAIN"), Scheme: get("CP_DEMO_SCHEME", "https"),
		ServerImage: need("CP_SERVER_IMAGE"), WebImage: need("CP_WEB_IMAGE"),
		Memory: get("CP_DEMO_MEMORY", "384m"), CPUs: get("CP_DEMO_CPUS", "0.5"),
		PGAdminURL: need("CP_PG_ADMIN_URL"), PGHost: need("CP_PG_HOST"), PGUser: get("CP_PG_USER", "erp_demo"), PGPassword: need("CP_PG_PASSWORD"),
		Template: get("CP_DEMO_TEMPLATE", "erp_demo_seed"), RedisURL: need("CP_REDIS_URL"), DataNetwork: get("CP_DATA_NETWORK", "erp-data"),
		ProxyNetwork: get("CP_PROXY_NETWORK", "erp-proxy"), Entrypoint: get("CP_TRAEFIK_ENTRYPOINT", "websecure"), CertResolver: certResolver(get("CP_TRAEFIK_CERT_RESOLVER", "letsencrypt")),
		EntitlementTemplate: get("CP_DEMO_ENTITLEMENT_TEMPLATE", "agency"),
		AIProvider:          get("CP_DEMO_AI_PROVIDER", ""), AIModel: get("CP_DEMO_AI_MODEL", ""), AIKey: get("CP_DEMO_AI_API_KEY", ""), AIMaxQuestions: num("CP_DEMO_AI_MAX_QUESTIONS", 15),
	}
	if c.Demo.AIKey != "" && (c.Demo.AIProvider == "" || c.Demo.AIModel == "") {
		problems = append(problems, "CP_DEMO_AI_PROVIDER and CP_DEMO_AI_MODEL: both are required when CP_DEMO_AI_API_KEY is set")
	}
	if c.Demo.Scheme != "https" && c.Demo.Scheme != "http" {
		problems = append(problems, "CP_DEMO_SCHEME: must be https or http")
	}
	c.Install = Install{
		PublicURL: strings.TrimRight(get("CP_PUBLIC_URL", ""), "/"), Registry: strings.TrimRight(get("CP_INSTALL_REGISTRY", ""), "/"),
		User: get("CP_INSTALL_REGISTRY_USER", ""), Token: get("CP_INSTALL_REGISTRY_TOKEN", ""),
	}
	// These are written into a shell script the customer runs, so they may hold nothing a shell would act on.
	for key, v := range map[string]string{"CP_INSTALL_REGISTRY": c.Install.Registry, "CP_INSTALL_REGISTRY_USER": c.Install.User, "CP_INSTALL_REGISTRY_TOKEN": c.Install.Token} {
		if v != "" && !ShellSafe(v) {
			problems = append(problems, key+": may hold only letters, digits and . _ : / @ + -")
		}
	}
	if u := c.Install.PublicURL; u != "" && !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		problems = append(problems, "CP_PUBLIC_URL: must be an http(s) address")
	}
	if (c.Install.User == "") != (c.Install.Token == "") {
		problems = append(problems, "CP_INSTALL_REGISTRY_USER and CP_INSTALL_REGISTRY_TOKEN: set both, or neither when the images are public")
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("config: %w", errors.New(strings.Join(problems, "; ")))
	}
	return c, nil
}

// certResolver turns "none" into no resolver: demos are then served over plain HTTP, for a laptop.
func certResolver(v string) string {
	if v == "none" {
		return ""
	}
	return v
}

// ShellSafe says whether v is made only of characters that mean nothing to a shell (letters, digits and . _ : / @ + -).
func ShellSafe(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case strings.ContainsRune("._:/@+-", r):
		default:
			return false
		}
	}
	return v != ""
}
