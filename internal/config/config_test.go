package config

import (
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func base() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN":    "tok",
		"DEVIN_API_KEY":    "cog_x",
		"DEVIN_ORG_ID":     "org-1",
		"ALLOWED_USER_IDS": "123, 456",
	}
}

func TestParseDefaults(t *testing.T) {
	cfg, err := parse(env(base()))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DevinAPIURL != DefaultDevinAPIURL || cfg.PollInterval != 15*time.Second || cfg.StorePath != "devincord.json" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.Allowed(snowflake.ID(456)) || cfg.Allowed(snowflake.ID(789)) {
		t.Fatalf("allowlist not applied: %v", cfg.AllowedUsers)
	}
	if len(cfg.DevinTags) != 1 || cfg.DevinTags[0] != "discord" {
		t.Fatalf("default tags: %v", cfg.DevinTags)
	}
	if len(cfg.GuildIDs()) != 0 || !cfg.SyncCommands {
		t.Fatalf("guild/sync defaults wrong")
	}
}

func TestParseMissingRequired(t *testing.T) {
	_, err := parse(env(map[string]string{"ALLOWED_USER_IDS": "1"}))
	if err == nil || !strings.Contains(err.Error(), "DEVIN_API_KEY, DEVIN_ORG_ID, DISCORD_TOKEN") {
		t.Fatalf("got %v", err)
	}
}

func TestParseRequiresAllowlist(t *testing.T) {
	m := base()
	delete(m, "ALLOWED_USER_IDS")
	if _, err := parse(env(m)); err == nil {
		t.Fatal("expected error without ALLOWED_USER_IDS")
	}
}

func TestParseOverrides(t *testing.T) {
	m := base()
	m["POLL_INTERVAL"] = "30s"
	m["DEVIN_API_URL"] = "http://localhost:9/"
	m["MAX_ACU_LIMIT"] = "5"
	m["SYNC_COMMANDS"] = "no"
	m["DISCORD_GUILD_ID"] = "42"
	m["DEVIN_TAGS"] = "a,b"
	cfg, err := parse(env(m))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollInterval != 30*time.Second || cfg.DevinAPIURL != "http://localhost:9" || cfg.MaxACULimit != 5 ||
		cfg.SyncCommands || cfg.GuildID != 42 || len(cfg.DevinTags) != 2 {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
}

func TestParseRejectsBadValues(t *testing.T) {
	for key, val := range map[string]string{
		"POLL_INTERVAL":    "1s",
		"MAX_ACU_LIMIT":    "-1",
		"DISCORD_GUILD_ID": "abc",
		"ALLOWED_USER_IDS": "x",
	} {
		m := base()
		m[key] = val
		if _, err := parse(env(m)); err == nil {
			t.Errorf("%s=%q: expected error", key, val)
		}
	}
}
