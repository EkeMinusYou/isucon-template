package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	measurement "github.com/EkeMinusYou/isucon-template/tools/measurement/app"
)

type serverConfig struct {
	Address         string
	MeasurementAddr string
	Snapshot        func() measurement.Snapshot
}

func serve(ctx context.Context, config serverConfig, handler http.Handler) error {
	// Diagnostics remain private even when application traffic crosses hosts.
	host, _, err := net.SplitHostPort(config.MeasurementAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("MEASUREMENT_ADDR must bind a loopback IP: %q", config.MeasurementAddr)
	}
	listener, err := net.Listen("tcp", config.Address)
	if err != nil {
		return fmt.Errorf("application listen: %w", err)
	}
	defer listener.Close()
	metrics, err := measurement.Start(measurement.Options{
		Address: config.MeasurementAddr, Snapshot: config.Snapshot,
	})
	if err != nil {
		return err
	}
	defer metrics.Close()

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		// Whole-request read/write deadlines are deliberately unset: configure
		// them per route when the contest's upload and streaming rules are known.
	}
	defer server.Close()
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	log.Printf("application listening on %s; measurements on %s", listener.Addr(), metrics.Addr)

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("application serve: %w", err)
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("application shutdown: %w", err)
		}
		return nil
	}
}
