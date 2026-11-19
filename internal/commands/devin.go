// Package commands implements the /devin slash command and thread message forwarding.
package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"

	"github.com/atluixx/devincord/internal/config"
	"github.com/atluixx/devincord/internal/devin"
	"github.com/atluixx/devincord/internal/relay"
	"github.com/atluixx/devincord/internal/store"
)

const accent = 0x2F6FEB

var Definitions = []discord.ApplicationCommandCreate{
	discord.SlashCommandCreate{
		Name:        "devin",
		Description: "Start and follow Devin sessions",
		Options: []discord.ApplicationCommandOption{
			discord.ApplicationCommandOptionSubCommand{
				Name:        "start",
				Description: "Start a Devin session in a new thread",
				Options: []discord.ApplicationCommandOption{
					discord.ApplicationCommandOptionString{Name: "prompt", Description: "What Devin should do", Required: true, MaxLength: intPtr(4000)},
					discord.ApplicationCommandOptionString{Name: "title", Description: "Thread and session title", MaxLength: intPtr(90)},
				},
			},
			discord.ApplicationCommandOptionSubCommand{
				Name:        "status",
				Description: "List your recent Devin sessions",
			},
		},
	},
}

type DevinAPI interface {
	relay.API
	CreateSession(ctx context.Context, req devin.CreateSessionRequest) (devin.Session, error)
	SendMessage(ctx context.Context, sessionID, message string) (devin.Session, error)
}

type Handlers struct {
	cfg   config.Config
	api   DevinAPI
	store *store.Store
	relay *relay.Relay
}

func New(cfg config.Config, api DevinAPI, st *store.Store, r *relay.Relay) *Handlers {
	return &Handlers{cfg: cfg, api: api, store: st, relay: r}
}

func (h *Handlers) Register(r handler.Router) {
	r.SlashCommand("/devin/start", h.start)
	r.SlashCommand("/devin/status", h.status)
}

func (h *Handlers) start(data discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	user := e.User()
	if !h.cfg.Allowed(user.ID) {
		return e.CreateMessage(ephemeral("You are not allowed to start Devin sessions."))
	}
	channel := e.Channel()
	if e.GuildID() == nil || channel.MessageChannel == nil ||
		(channel.Type() != discord.ChannelTypeGuildText && channel.Type() != discord.ChannelTypeGuildNews) {
		return e.CreateMessage(ephemeral("Use /devin start in a server text channel, not a thread or DM."))
	}

	prompt := strings.TrimSpace(data.String("prompt"))
	title := strings.TrimSpace(data.String("title"))
	if err := e.DeferCreateMessage(false); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	session, err := h.api.CreateSession(ctx, devin.CreateSessionRequest{
		Prompt:      prompt,
		Title:       title,
		Tags:        h.cfg.DevinTags,
		MaxACULimit: h.cfg.MaxACULimit,
	})
	if err != nil {
		slog.Error("create session failed", slog.Any("err", err))
		_, uerr := e.UpdateInteractionResponse(discord.MessageUpdate{Content: strPtr("Could not start a Devin session: " + err.Error())})
		return uerr
	}

	embed := discord.NewEmbed().
		WithTitle(orDefault(title, "Devin session")).
		WithURL(session.URL).
		WithDescription(truncate(prompt, 3500)).
		WithColor(accent).
		WithFooterText("Started by " + user.Username + " · " + session.SessionID)
	msg, err := e.UpdateInteractionResponse(discord.MessageUpdate{Embeds: &[]discord.Embed{embed}})
	if err != nil {
		return err
	}

	thread, err := e.Client().Rest.CreateThreadFromMessage(channel.ID(), msg.ID, discord.ThreadCreateFromMessage{
		Name:                ThreadName(title, prompt),
		AutoArchiveDuration: discord.AutoArchiveDuration1w,
	})
	if err != nil {
		slog.Error("create thread failed", slog.Any("err", err))
		_, ferr := e.CreateFollowupMessage(discord.MessageCreate{Content: "Session started, but I could not open a thread: " + session.URL})
		return ferr
	}

	link := store.Link{
		ThreadID:  thread.ID().String(),
		SessionID: session.SessionID,
		URL:       session.URL,
		Title:     title,
		OwnerID:   user.ID.String(),
		CreatedAt: time.Now(),
		Status:    session.Status,
		Active:    true,
	}
	if err := h.store.Put(link); err != nil {
		slog.Error("save link failed", slog.Any("err", err))
	}
	_, err = e.Client().Rest.CreateMessage(thread.ID(), discord.MessageCreate{
		Content:         "Session started: " + session.URL + "\nMessages you send in this thread go to Devin. Devin's replies will show up here.",
		AllowedMentions: &discord.AllowedMentions{},
	})
	h.syncSoon(link.ThreadID)
	return err
}

func (h *Handlers) syncSoon(threadID string) {
	go func() {
		time.Sleep(5 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := h.relay.Sync(ctx, threadID); err != nil {
			slog.Warn("relay sync failed", slog.Any("err", err))
		}
	}()
}

func (h *Handlers) status(_ discord.SlashCommandInteractionData, e *handler.CommandEvent) error {
	links := h.store.ByOwner(e.User().ID.String())
	if len(links) == 0 {
		return e.CreateMessage(ephemeral("You have no Devin sessions yet. Start one with /devin start."))
	}
	var b strings.Builder
	for i, l := range links {
		if i == 10 {
			break
		}
		state := l.Status
		if l.StatusDetail != "" {
			state += " / " + l.StatusDetail
		}
		fmt.Fprintf(&b, "<#%s> · [%s](%s) · %s\n", l.ThreadID, orDefault(l.Title, l.SessionID), l.URL, orDefault(state, "unknown"))
	}
	embed := discord.NewEmbed().WithTitle("Your Devin sessions").WithDescription(b.String()).WithColor(accent)
	return e.CreateMessage(discord.MessageCreate{Embeds: []discord.Embed{embed}, Flags: discord.MessageFlagEphemeral})
}

// OnMessage forwards messages posted in a session thread to that Devin session.
func (h *Handlers) OnMessage(e *events.GuildMessageCreate) {
	m := e.Message
	if m.Author.Bot || m.Author.System {
		return
	}
	link, ok := h.store.Get(e.ChannelID.String())
	if !ok {
		return
	}
	rest := e.Client().Rest
	if !h.cfg.Allowed(m.Author.ID) {
		_ = rest.AddReaction(e.ChannelID, m.ID, "🔒")
		return
	}
	text := MessageText(m)
	if text == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := h.api.SendMessage(ctx, link.SessionID, text); err != nil {
		slog.Error("send message failed", slog.String("session", link.SessionID), slog.Any("err", err))
		_, _ = rest.CreateMessage(e.ChannelID, discord.MessageCreate{
			Content:          "Could not send that to Devin: " + err.Error(),
			MessageReference: &discord.MessageReference{MessageID: &m.ID},
			AllowedMentions:  &discord.AllowedMentions{},
		})
		return
	}
	_ = rest.AddReaction(e.ChannelID, m.ID, "✅")
	if _, _, err := h.store.Update(link.ThreadID, func(l *store.Link) { l.Active = true }); err != nil {
		slog.Error("update link failed", slog.Any("err", err))
	}
	h.syncSoon(link.ThreadID)
}

// Poster posts relay output to Discord threads.
type Poster struct{ Client *bot.Client }

func (p Poster) Post(threadID, content string) error {
	id, err := snowflake.Parse(threadID)
	if err != nil {
		return err
	}
	_, err = p.Client.Rest.CreateMessage(id, discord.MessageCreate{Content: content, AllowedMentions: &discord.AllowedMentions{}})
	return err
}

// MessageText is the text sent to Devin for a Discord message, including attachment links.
func MessageText(m discord.Message) string {
	parts := []string{strings.TrimSpace(m.Content)}
	for _, a := range m.Attachments {
		parts = append(parts, a.URL)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// ThreadName builds a Discord thread name (max 100 chars) from the title or prompt.
func ThreadName(title, prompt string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = strings.Join(strings.Fields(prompt), " ")
	}
	if name == "" {
		name = "Devin session"
	}
	return truncate(name, 100)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func ephemeral(content string) discord.MessageCreate {
	return discord.MessageCreate{Content: content, Flags: discord.MessageFlagEphemeral}
}

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }
