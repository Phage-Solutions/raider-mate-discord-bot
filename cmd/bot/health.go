package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
)

// healthHandler answers liveness checks. It runs no diagnostics: by the time run()
// starts this server, Start has already opened the gateway, so 200 means the bot
// reached a running state, not that Discord is currently reachable.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// startHealthServer opens a listener Scaleway's container health check can probe. The
// bot itself accepts no inbound traffic; this exists only because the platform
// requires a port to watch.
func startHealthServer(addr string, logger *slog.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthHandler)
	srv := &http.Server{Addr: addr, Handler: mux}

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("health server", "error", err)
		}
	}()

	return srv
}

func stopHealthServer(ctx context.Context, srv *http.Server, logger *slog.Logger) {
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("closing health server", "error", err)
	}
}
