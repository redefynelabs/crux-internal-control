package config

import (
	"strings"
	"testing"
)

func env() map[string]string {
	return map[string]string{
		"CP_DATABASE_URL": "postgres://x", "CP_SECRETS_KEY": strings.Repeat("t", 40), "CP_DEMO_DOMAIN": "demo.example.com",
		"CP_SERVER_IMAGE": "s:1", "CP_WEB_IMAGE": "w:1", "CP_PG_ADMIN_URL": "postgres://a", "CP_PG_HOST": "pg:5432", "CP_PG_PASSWORD": "pw", "CP_REDIS_URL": "redis://r",
	}
}

func loadFrom(m map[string]string) (*Config, error) {
	return load(func(k string) (string, bool) { v, ok := m[k]; return v, ok })
}

func TestDefaults(t *testing.T) {
	c, err := loadFrom(env())
	if err != nil {
		t.Fatal(err)
	}
	if c.Demo.TTL.Hours() != 1 || c.Demo.MaxLive != 20 || c.Demo.MaxPerEmail != 2 || c.Demo.Scheme != "https" || c.Demo.Template != "erp_demo_seed" {
		t.Fatalf("unexpected defaults: %+v", c.Demo)
	}
}

func TestRequiredSettingsAreNamedTogether(t *testing.T) {
	_, err := loadFrom(map[string]string{})
	if err == nil {
		t.Fatal("an empty environment must fail")
	}
	for _, k := range []string{"CP_DATABASE_URL", "CP_SECRETS_KEY", "CP_DEMO_DOMAIN", "CP_SERVER_IMAGE", "CP_PG_PASSWORD"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("the error should name %s: %v", k, err)
		}
	}
}

func TestBadValuesAreRefusedWithoutEchoingSecrets(t *testing.T) {
	for key, bad := range map[string]string{"CP_SECRETS_KEY": "short", "CP_DEMO_TTL": "forever", "CP_MAX_DEMOS": "0", "CP_DEMO_SCHEME": "ftp"} {
		e := env()
		e[key] = bad
		_, err := loadFrom(e)
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q should be refused, got %v", key, bad, err)
		}
		if err != nil && strings.Contains(err.Error(), "pw") && key == "CP_DEMO_TTL" {
			t.Errorf("an error must not echo secrets: %v", err)
		}
	}
}
