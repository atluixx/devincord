package config

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
	"github.com/joho/godotenv"
)

const DefaultDevinAPIURL = "https://api.devin.ai"

type Config struct {
	Token        string
	GuildID      snowflake.ID
	SyncCommands bool
	LogLevel     slog.Level

	DevinAPIKey  string
	DevinOrgID   string
	DevinAPIURL  string
	DevinTags    []string
	MaxACULimit  int
	AllowedUsers map[snowflake.ID]bool
	PollInterval time.Duration
	StorePath    string
}

func Load() (Config, error) {
	_ = godotenv.Load()
	return parse(os.Getenv)
}

func parse(getenv func(string) string) (Config, error) {
	get := func(key string) string { return strings.TrimSpace(getenv(key)) }

	cfg := Config{
		Token:        get("DISCORD_TOKEN"),
		SyncCommands: true,
		LogLevel:     slog.LevelInfo,
		DevinAPIKey:  get("DEVIN_API_KEY"),
		DevinOrgID:   get("DEVIN_ORG_ID"),
		DevinAPIURL:  strings.TrimRight(get("DEVIN_API_URL"), "/"),
		DevinTags:    splitList(get("DEVIN_TAGS")),
		AllowedUsers: map[snowflake.ID]bool{},
		PollInterval: 15 * time.Second,
		StorePath:    get("STORE_PATH"),
	}
	if cfg.DevinAPIURL == "" {
		cfg.DevinAPIURL = DefaultDevinAPIURL
	}
	if cfg.StorePath == "" {
		cfg.StorePath = "devincord.json"
	}
	if get("DEVIN_TAGS") == "" {
		cfg.DevinTags = []string{"discord"}
	}

	var missing []string
	for key, val := range map[string]string{
		"DISCORD_TOKEN": cfg.Token,
		"DEVIN_API_KEY": cfg.DevinAPIKey,
		"DEVIN_ORG_ID":  cfg.DevinOrgID,
	} {
		if val == "" {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Config{}, fmt.Errorf("missing required settings: %s", strings.Join(missing, ", "))
	}

	for _, raw := range splitList(get("ALLOWED_USER_IDS")) {
		id, err := snowflake.Parse(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid ALLOWED_USER_IDS entry %q: %w", raw, err)
		}
		cfg.AllowedUsers[id] = true
	}
	if len(cfg.AllowedUsers) == 0 {
		return Config{}, fmt.Errorf("ALLOWED_USER_IDS is required: sessions spend your Devin ACUs")
	}

	if raw := get("DISCORD_GUILD_ID"); raw != "" {
		id, err := snowflake.Parse(raw)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DISCORD_GUILD_ID %q: %w", raw, err)
		}
		cfg.GuildID = id
	}

	if raw := get("SYNC_COMMANDS"); strings.EqualFold(raw, "0") ||
		strings.EqualFold(raw, "false") || strings.EqualFold(raw, "no") {
		cfg.SyncCommands = false
	}

	if raw := get("POLL_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d < 5*time.Second {
			return Config{}, fmt.Errorf("invalid POLL_INTERVAL %q: use a duration of at least 5s", raw)
		}
		cfg.PollInterval = d
	}

	if raw := get("MAX_ACU_LIMIT"); raw != "" {
		var n int
		if _, err := fmt.Sscanf(raw, "%d", &n); err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid MAX_ACU_LIMIT %q: use a positive integer", raw)
		}
		cfg.MaxACULimit = n
	}

	switch strings.ToLower(get("LOG_LEVEL")) {
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	}

	return cfg, nil
}

func (c Config) GuildIDs() []snowflake.ID {
	if c.GuildID == 0 {
		return nil
	}
	return []snowflake.ID{c.GuildID}
}

func (c Config) Allowed(id snowflake.ID) bool {
	return c.AllowedUsers[id]
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
