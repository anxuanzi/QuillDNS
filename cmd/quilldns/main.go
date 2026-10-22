package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/anxuanzi/QuillDNS/internal/dnsserver"
	"github.com/anxuanzi/QuillDNS/internal/httpapi"
	"github.com/anxuanzi/QuillDNS/internal/observability"
	"github.com/anxuanzi/QuillDNS/internal/records"
	"github.com/anxuanzi/QuillDNS/webassets"
	"github.com/miekg/dns"
)

func main() {
	var data, listen, dnsListen, upstream string
	flag.StringVar(&data, "data", "./data/records.json", "persistent override file")
	flag.StringVar(&listen, "http", httpapi.ListenAddress(), "management bind address (keep private until OIDC is implemented)")
	flag.StringVar(&dnsListen, "dns", "127.0.0.1:1053", "DNS listener (UDP and TCP)")
	flag.StringVar(&upstream, "upstreams", "1.1.1.1:53,8.8.8.8:53", "comma-separated DNS upstreams")
	flag.Parse()

	store, err := records.Open(data)
	if err != nil {
		slog.Error("open records", "error", err)
		os.Exit(1)
	}
	telemetry := observability.New(2000)
	server := dnsserver.New(store, telemetry, strings.Split(upstream, ","))
	dns.HandleFunc(".", server.ServeDNS)
	udp := &dns.Server{Addr: dnsListen, Net: "udp"}
	tcp := &dns.Server{Addr: dnsListen, Net: "tcp"}
	errch := make(chan error, 3)
	go func() { errch <- udp.ListenAndServe() }()
	go func() { errch <- tcp.ListenAndServe() }()

	api := (&httpapi.API{Records: store, DNS: server, Telemetry: telemetry}).Handler()
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/", webassets.Handler())
	httpServer := &http.Server{Addr: listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { errch <- httpServer.ListenAndServe() }()
	slog.Info("QuillDNS started", "dns", dnsListen, "http", listen)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	select {
	case <-ctx.Done():
	case err := <-errch:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server stopped", "error", err)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	_ = udp.Shutdown()
	_ = tcp.Shutdown()
}
