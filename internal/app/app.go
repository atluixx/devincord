package app

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/handler/middleware"

	"github.com/atluixx/devincord/internal/commands"
	"github.com/atluixx/devincord/internal/config"
	"github.com/atluixx/devincord/internal/devin"
	"github.com/atluixx/devincord/internal/relay"
	"github.com/atluixx/devincord/internal/store"
)

func Run() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid config", slog.Any("err", err))
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	st, err := store.Open(cfg.StorePath)
	if err != nil {
		slog.Error("failed to open store", slog.Any("err", err))
		os.Exit(1)
	}
	api := devin.New(cfg.DevinAPIURL, cfg.DevinOrgID, cfg.DevinAPIKey)

	var h *commands.Handlers
	router := handler.New()
	router.Use(middleware.Go)
	client, err := disgo.New(cfg.Token,
		bot.WithDefaultGateway(),
		bot.WithGatewayConfigOpts(gateway.WithIntents(
			gateway.IntentGuilds,
			gateway.IntentGuildMessages,
			gateway.IntentMessageContent,
		)),
		bot.WithEventListeners(router),
		bot.WithEventListenerFunc(func(e *events.GuildMessageCreate) { go h.OnMessage(e) }),
	)
	if err != nil {
		slog.Error("failed to build client", slog.Any("err", err))
		os.Exit(1)
	}
	defer client.Close(context.Background())

	rl := relay.New(api, st, commands.Poster{Client: client})
	h = commands.New(cfg, api, st, rl)
	h.Register(router)

	if cfg.SyncCommands {
		if err := handler.SyncCommands(client, commands.Definitions, cfg.GuildIDs()); err != nil {
			slog.Error("failed to sync commands", slog.Any("err", err))
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.OpenGateway(ctx); err != nil {
		slog.Error("failed to open gateway", slog.Any("err", err))
		os.Exit(1)
	}

	relayCtx, stopRelay := context.WithCancel(context.Background())
	relayDone := make(chan struct{})
	go func() { defer close(relayDone); rl.Run(relayCtx, cfg.PollInterval) }()
	defer func() { stopRelay(); <-relayDone }()

	slog.Info("devincord is running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM)
	<-s
}
