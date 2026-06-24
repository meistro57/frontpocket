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
	if err := run(os.Args[1:]); err != nil {
		slog.Error("frontpocket command failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && args[0] == "ingest" {
		return runIngestCommand(args[1:])
	}

	flags := flag.NewFlagSet("frontpocket", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		printRootHelp(flags)
	}
	showVersion := flags.Bool("version", false, "print FrontPocket version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println(version.Current)
		return nil
	}

	return runServer()
}

func printRootHelp(flags *flag.FlagSet) {
	fmt.Fprintln(flags.Output(), "Usage:")
	fmt.Fprintln(flags.Output(), "  frontpocket [command] [options]")
	fmt.Fprintln(flags.Output(), "  frontpocket --version")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Commands:")
	fmt.Fprintln(flags.Output(), "  ingest      Import memory data from supported sources.")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Subcommands:")
	fmt.Fprintln(flags.Output(), "  ingest chatgpt      Import from a ChatGPT export zip or folder.")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Help:")
	fmt.Fprintln(flags.Output(), "  frontpocket --help")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest --help")
	fmt.Fprintln(flags.Output(), "  frontpocket ingest chatgpt --help")
	fmt.Fprintln(flags.Output())
	fmt.Fprintln(flags.Output(), "Options:")
	flags.PrintDefaults()
}

func runServer() error {
	if err := config.LoadDotEnv(".env"); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logfp.New(cfg.Logging)
	server, err := api.NewServer(cfg, logger)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
