package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/pprof"
)

const pprofAddr = "127.0.0.1:6060"

func startPprofServer(enabled bool) *http.Server {
	if !enabled {
		return nil
	}

	server := newPprofServer()
	go func() {
		slog.Info("pprof server started", slog.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("pprof server failed", slog.Any("error", err))
		}
	}()
	return server
}

func newPprofServer() *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:    pprofAddr,
		Handler: mux,
	}
}
