package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/meistro57/frontpocket/internal/api"
	"github.com/meistro57/frontpocket/internal/config"
	logfp "github.com/meistro57/frontpocket/internal/log"
	"github.com/meistro57/frontpocket/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print FrontPocket version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Current)
		return
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		slog.Error("failed loading .env", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed loading config", "error", err)
		os.Exit(1)
	}

	logger := logfp.New(cfg.Logging)
	server, err := api.NewServer(cfg, logger)
	if err != nil {
		logger.Error("failed creating server", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("server terminated", "error", err)
		os.Exit(1)
	}
}
