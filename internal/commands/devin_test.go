package commands

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
)

func TestThreadName(t *testing.T) {
	if got := ThreadName("  Fix login ", "ignored"); got != "Fix login" {
		t.Fatalf("got %q", got)
	}
	if got := ThreadName("", "fix\n  the   bug"); got != "fix the bug" {
		t.Fatalf("got %q", got)
	}
	if got := ThreadName("", ""); got != "Devin session" {
		t.Fatalf("got %q", got)
	}
	long := ThreadName("", strings.Repeat("ação ", 50))
	if utf8.RuneCountInString(long) != 100 || !strings.HasSuffix(long, "…") {
		t.Fatalf("got %d runes: %q", utf8.RuneCountInString(long), long)
	}
}

func TestMessageTextIncludesAttachments(t *testing.T) {
	m := discord.Message{Content: " see this ", Attachments: []discord.Attachment{{URL: "https://cdn/x.png"}}}
	if got := MessageText(m); got != "see this\nhttps://cdn/x.png" {
		t.Fatalf("got %q", got)
	}
	if got := MessageText(discord.Message{}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestDefinitions(t *testing.T) {
	cmd, ok := Definitions[0].(discord.SlashCommandCreate)
	if !ok || cmd.Name != "devin" || len(cmd.Options) != 2 {
		t.Fatalf("unexpected definitions: %+v", Definitions)
	}
}
