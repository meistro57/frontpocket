// filename: cmd/minddrill/main.go
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed index.html
var indexHTML embed.FS

func main() {
	flags := flag.NewFlagSet("minddrill", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	port := flags.Int("port", 8089, "port to serve MindDrill UI on")
	api := flags.String("api", "http://localhost:8088", "FrontPocket API base URL")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: minddrill [options]")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "  Serves the MindDrill memory explorer UI in your browser.")
		fmt.Fprintln(flags.Output(), "  Requires FrontPocket API to be running.")
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Options:")
		flags.PrintDefaults()
		fmt.Fprintln(flags.Output())
		fmt.Fprintln(flags.Output(), "Examples:")
		fmt.Fprintln(flags.Output(), "  minddrill")
		fmt.Fprintln(flags.Output(), "  minddrill --port 9000")
		fmt.Fprintln(flags.Output(), "  minddrill --api http://localhost:8088")
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		slog.Error("minddrill flag error", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		data, err := indexHTML.ReadFile("index.html")
		if err != nil {
			http.Error(w, "could not read index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","app":"minddrill"}`)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", *port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	addr := fmt.Sprintf("http://localhost:%d", *port)
	slog.Info("MindDrill running", "url", addr, "api", *api)
	fmt.Printf("\n  MindDrill is ready\n  Open: %s\n  API:  %s\n\n", addr, *api)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("minddrill server error", "error", err)
		os.Exit(1)
	}
}
