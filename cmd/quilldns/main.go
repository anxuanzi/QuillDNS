package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/auth"
	"github.com/anxuanzi/QuillDNS/internal/config"
	"github.com/anxuanzi/QuillDNS/internal/dnsserver"
	"github.com/anxuanzi/QuillDNS/internal/httpapi"
	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/anxuanzi/QuillDNS/internal/settings"
	"github.com/anxuanzi/QuillDNS/webassets"
	"github.com/miekg/dns"
)

func main() {
	cfg, err := config.Parse(os.Args[1:], os.LookupEnv)
	if err != nil {
		slog.Error("invalid QuillDNS configuration", "error", err)
		os.Exit(2)
	}
	data, listen, dnsListen, upstream, allowed := cfg.DataPath, cfg.HTTPListen, cfg.DNSListen, cfg.Upstreams, cfg.AllowNetworks

	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		slog.Error("invalid management listener", "error", err)
		os.Exit(1)
	}
	if os.Getenv("QUILLDNS_OIDC_ISSUER") == "" && host != "localhost" && host != "127.0.0.1" && host != "::1" {
		slog.Error("unauthenticated management UI must bind to loopback")
		os.Exit(1)
	}
	store, err := records.Open(data)
	if err != nil {
		slog.Error("open records", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logsPath := filepath.Join(filepath.Dir(data), "queries.sqlite")
	telemetry, err := observability.Open(logsPath, 2000, cfg.LogRetentionDays, cfg.LogMaxRows)
	if err != nil {
		slog.Error("open telemetry", "error", err)
		os.Exit(1)
	}
	resolverSettings, err := settings.Open(data)
	if err != nil {
		slog.Error("open settings", "error", err)
		os.Exit(1)
	}
	defer resolverSettings.Close()
	addresses, err := resolverSettings.Upstreams(strings.Split(upstream, ","))
	if err != nil {
		slog.Error("invalid persisted upstream configuration", "error", err)
		os.Exit(1)
	}
	server := dnsserver.New(store, telemetry, addresses)
	if err = server.SetUpstreams(addresses); err != nil {
		slog.Error("invalid resolver upstreams", "error", err)
		os.Exit(1)
	}
	if err := server.SetAllowedNetworks(allowed); err != nil {
		slog.Error("invalid client access policy", "error", err)
		os.Exit(1)
	}
	muxDNS := dns.NewServeMux()
	muxDNS.Handle(".", server)
	udp := &dns.Server{Addr: dnsListen, Net: "udp", Handler: muxDNS}
	tcp := &dns.Server{Addr: dnsListen, Net: "tcp", Handler: muxDNS}
	errch := make(chan error, 3)
	go func() { errch <- udp.ListenAndServe() }()
	go func() { errch <- tcp.ListenAndServe() }()

	api := (&httpapi.API{Records: store, DNS: server, Telemetry: telemetry, Settings: resolverSettings}).Handler()
	var oidcManager *auth.Manager
	oidcSettings := auth.Config{
		Issuer:        os.Getenv("QUILLDNS_OIDC_ISSUER"),
		ClientID:      os.Getenv("QUILLDNS_OIDC_CLIENT_ID"),
		ClientSecret:  os.Getenv("QUILLDNS_OIDC_CLIENT_SECRET"),
		PublicURL:     os.Getenv("QUILLDNS_PUBLIC_URL"),
		AllowedEmails: strings.Split(os.Getenv("QUILLDNS_OIDC_ALLOWED_EMAILS"), ","),
	}
	if oidcSettings.Issuer != "" || oidcSettings.ClientID != "" || oidcSettings.ClientSecret != "" || oidcSettings.PublicURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		oidcManager, err = auth.New(ctx, oidcSettings)
		cancel()
		if err != nil {
			slog.Error("initialize OIDC", "error", err)
			os.Exit(1)
		}
	}
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("GET /metrics", api)
	mux.Handle("/", webassets.Handler())
	if oidcManager != nil {
		oidcManager.Routes(mux)
	} else {
		mux.HandleFunc("GET /api/v1/session", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(`{"mode":"disabled","authenticated":false}`))
		})
	}
	var handler http.Handler = mux
	if oidcManager != nil {
		handler = oidcManager.Guard(mux)
	}
	httpServer := &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() { errch <- httpServer.ListenAndServe() }()
	slog.Info("QuillDNS started", "dns", dnsListen, "http", listen, "allowed_networks", allowed)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errch:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	_ = udp.Shutdown()
	_ = tcp.Shutdown()
	if err := telemetry.Close(shutdown); err != nil {
		slog.Warn("log flush incomplete", "error", err)
	}
}
