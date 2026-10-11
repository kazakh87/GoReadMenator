package cli

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"goreadmenator/internal/httpapi"
)

// cmdServe startet die HTTP-Schnittstelle. Sie ist nur lesend. Standardmäßig bindet sie an
// 127.0.0.1. Ein Token schützt zusätzlich, wenn sie über das Netz erreichbar gemacht wird.
func (a *App) cmdServe(ctx context.Context, addr, token string) int {
	log := slog.New(slog.NewTextHandler(a.Stderr, nil))
	if token == "" {
		token = os.Getenv("READMENATOR_TOKEN")
	}
	mgr, err := a.projectManager(ctx, log)
	if err != nil {
		log.Error("Projekte laden", "err", err)
		return ExitError
	}
	defer mgr.closeAll()

	if host, _, err := net.SplitHostPort(addr); err == nil && !isLoopback(host) {
		if token == "" {
			log.Warn("Schnittstelle ist über das Netz erreichbar und hat kein Token. Mit -token oder READMENATOR_TOKEN absichern.", "addr", addr)
		}
	}
	srv := httpapi.New(mgr.resolve, mgr.names, token)
	hs := &http.Server{Addr: addr, Handler: srv, ReadHeaderTimeout: 5 * time.Second}

	errc := make(chan error, 1)
	go func() { errc <- hs.ListenAndServe() }()
	log.Info("HTTP-Schnittstelle läuft", "addr", addr, "projekte", len(mgr.names()))

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("Server beendet", "err", err)
			return ExitError
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdownCtx)
	}
	return ExitOK
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
