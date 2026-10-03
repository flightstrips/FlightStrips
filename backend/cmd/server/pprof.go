package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"strconv"
)

const pprofAddr = "127.0.0.1:6060"

func startPprofServer(enabled bool) *http.Server {
	if !enabled {
		return nil
	}

	address, mutexFraction, blockRate, err := pprofSettings(os.Getenv("PPROF_ADDR"), os.Getenv("PPROF_MUTEX_FRACTION"), os.Getenv("PPROF_BLOCK_RATE"))
	if err != nil {
		slog.Error("pprof configuration rejected", slog.Any("error", err))
		return nil
	}
	if mutexFraction > 0 {
		runtime.SetMutexProfileFraction(mutexFraction)
	}
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
	}
	server := newPprofServerAt(address)
	go func() {
		slog.Info("pprof server started", slog.String("address", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("pprof server failed", slog.Any("error", err))
		}
	}()
	return server
}

func newPprofServer() *http.Server {
	return newPprofServerAt(pprofAddr)
}

func pprofSettings(address, mutex, block string) (string, int, int, error) {
	if address == "" {
		address = pprofAddr
	}
	host, port, err := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || portNumber < 1 || portNumber > 65535 {
		return "", 0, 0, fmt.Errorf("pprof address requires a loopback IP and port between 1 and 65535")
	}
	values := [2]int{}
	for i, raw := range []string{mutex, block} {
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return "", 0, 0, fmt.Errorf("pprof sampling settings require nonnegative integers")
		}
		values[i] = value
	}
	return address, values[0], values[1], nil
}

func newPprofServerAt(address string) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	return &http.Server{
		Addr:    address,
		Handler: mux,
	}
}
