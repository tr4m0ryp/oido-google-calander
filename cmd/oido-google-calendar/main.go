// Command oido-google-calendar is a Google Calendar MCP server.
//
// With no arguments it speaks MCP over stdio using a service account, as
// upstream did. "serve" (or MCP_TRANSPORT=http) runs the multi-account HTTP
// service with the admin dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	_ "time/tzdata" // IANA zones even on minimal images without /usr/share/zoneinfo

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tr4m0ryp/oido-google-calander/internal/account"
	"github.com/tr4m0ryp/oido-google-calander/internal/auth/crypt"
	"github.com/tr4m0ryp/oido-google-calander/internal/auth/googleauth"
	"github.com/tr4m0ryp/oido-google-calander/internal/config"
	"github.com/tr4m0ryp/oido-google-calander/internal/server"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/admin"
	"github.com/tr4m0ryp/oido-google-calander/internal/server/clientip"
	"github.com/tr4m0ryp/oido-google-calander/internal/store"
	"github.com/tr4m0ryp/oido-google-calander/internal/tools"
)

// usage documents the modes; flags come after the mode keyword.
const usage = `usage: oido-google-calendar [--config JSON]   MCP over stdio (service account)
       oido-google-calendar serve             multi-account HTTP service
       oido-google-calendar healthcheck       probe a local HTTP service (exit 0 when healthy)`

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "2.0.0-dev"

func main() {
	legacy := flag.String("config", "", "JSON with CALENDAR_* settings (stdio mode; env vars win)")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, usage); flag.PrintDefaults() }
	flag.Parse()
	// Logs go to stderr: in stdio mode stdout carries the protocol.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch {
	case flag.Arg(0) == "healthcheck":
		err = healthcheck(ctx)
	case flag.Arg(0) == "serve" || os.Getenv("MCP_TRANSPORT") == "http":
		err = serveHTTP(ctx)
	default:
		err = serveStdio(ctx, *legacy)
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func serveStdio(ctx context.Context, legacyJSON string) error {
	if exe, err := os.Executable(); err == nil {
		if err := config.LoadDotEnv(filepath.Join(filepath.Dir(exe), ".env"), os.Setenv, os.Getenv); err != nil {
			return err
		}
	}
	if legacyJSON != "" {
		if err := config.ApplyLegacyJSON(legacyJSON, os.Setenv, os.Getenv); err != nil {
			return err
		}
	}
	cfg, err := config.LoadStdio(os.Getenv)
	if err != nil {
		return err
	}
	if cfg.ServiceAccountJSON == "" {
		return errors.New("stdio mode needs CALENDAR_SERVICE_ACCOUNT_JSON or CALENDAR_SERVICE_ACCOUNT_B64; " +
			"for OAuth accounts run the HTTP service with `serve`")
	}
	sa, err := account.NewServiceAccount(cfg.ServiceAccountJSON, cfg.CalendarID, cfg.DefaultZone)
	if err != nil {
		return err
	}
	slog.Info("serving MCP on stdio", "account", sa.Emails()[0], "calendar", cfg.CalendarID)
	return tools.NewServer(sa, version).Run(ctx, &mcp.StdioTransport{})
}

func serveHTTP(ctx context.Context) error {
	cfg, err := config.LoadHTTP(os.Getenv)
	if err != nil {
		return err
	}
	box, err := crypt.New(cfg.EncryptionKey)
	if err != nil {
		return err
	}
	accounts, err := store.Open(cfg.DataDir, box)
	if err != nil {
		return err
	}
	flow := googleauth.NewFlow(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.ServerURL+"/oauth/callback")
	provider := account.NewOAuth(accounts, flow.Config(), cfg.DefaultZone)
	adm, err := admin.New(admin.Deps{
		Config: cfg, Store: accounts, Accounts: provider, Flow: flow,
		ClientIP: clientip.New(cfg.TrustedProxies).Of, ToolCount: tools.Count,
	})
	if err != nil {
		return err
	}
	handler := server.New(server.Options{
		MCP: tools.NewServer(provider, version), APIKey: cfg.MCPAPIKey, Admin: adm,
		AccountCount: accounts.Len, Version: version,
	})
	srv := &http.Server{
		Addr: cfg.Addr, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 120 * time.Second, IdleTimeout: 120 * time.Second,
	}
	if cfg.MCPAPIKey == "" {
		slog.Warn("MCP_API_KEY is empty: /mcp is UNAUTHENTICATED")
	}
	slog.Info("serving", "addr", cfg.Addr, "mcp", cfg.ServerURL+"/mcp", "setup", cfg.ServerURL+"/setup",
		"accounts", accounts.Len(), "version", version)

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	select {
	case err := <-errc:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// healthcheck lets a distroless container probe itself without curl.
func healthcheck(ctx context.Context) error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/health", nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("health: HTTP %d", res.StatusCode)
	}
	return nil
}
