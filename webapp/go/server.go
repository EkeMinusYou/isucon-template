package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	measurement "github.com/EkeMinusYou/isucon-template/tools/measurement/app"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type serverConfig struct {
	Address         string
	TLSAddress      string
	TLSCertFile     string
	TLSKeyFile      string
	MeasurementAddr string
	AccessLog       string
	StaticDir       string
	StaticPrefix    string
	Snapshot        func() measurement.Snapshot
}

func serve(ctx context.Context, config serverConfig, handler http.Handler) error {
	host, _, err := net.SplitHostPort(config.MeasurementAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("MEASUREMENT_ADDR must bind a loopback IP: %q", config.MeasurementAddr)
	}
	if config.Address == "" && config.TLSAddress == "" {
		return errors.New("at least one public HTTP or TLS address is required")
	}
	if config.AccessLog == "" {
		return errors.New("ACCESS_LOG is required")
	}
	if err := os.MkdirAll(filepath.Dir(config.AccessLog), 0775); err != nil {
		return err
	}
	file, err := os.OpenFile(config.AccessLog, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0664)
	if err != nil {
		return err
	}
	defer file.Close()
	logger, err := measurement.NewAccessLogger(file, measurement.AccessLoggerOptions{})
	if err != nil {
		return err
	}
	defer logger.Close()
	observer := measurement.NewHTTPObserver(logger)
	metrics, err := measurement.Start(measurement.Options{Address: config.MeasurementAddr, Snapshot: func() measurement.Snapshot {
		snapshot := measurement.Snapshot{}
		if config.Snapshot != nil {
			snapshot = config.Snapshot()
		}
		combined := make(map[string]any, len(snapshot.Metrics)+5)
		for key, value := range snapshot.Metrics {
			combined[key] = value
		}
		for key, value := range logger.Metrics() {
			combined[key] = value
		}
		snapshot.Metrics = combined
		return snapshot
	}})
	if err != nil {
		return err
	}
	defer metrics.Close()
	if config.StaticDir != "" {
		static, err := newStaticHandler(config.StaticDir)
		if err != nil {
			return err
		}
		prefix := config.StaticPrefix
		if prefix == "" {
			prefix = "/static/"
		}
		if len(prefix) < 2 || prefix[0] != '/' || prefix[len(prefix)-1] != '/' {
			return errors.New("STATIC_URL_PREFIX must start and end with / and must not be /")
		}
		api := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(r.URL.Path) >= len(prefix) && r.URL.Path[:len(prefix)] == prefix {
				http.StripPrefix(prefix, static).ServeHTTP(w, r)
				return
			}
			api.ServeHTTP(w, r)
		})
	}
	handler = observer.Wrap(connectionPolicy(handler))
	var servers []*http.Server
	var listeners []net.Listener
	defer func() {
		for _, s := range servers {
			s.Close()
		}
		for _, l := range listeners {
			l.Close()
		}
	}()
	add := func(address string, secure bool) error {
		if address == "" {
			return nil
		}
		l, err := net.Listen("tcp", address)
		if err != nil {
			return fmt.Errorf("public listen: %w", err)
		}
		server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 120 * time.Second, ConnContext: observer.ConnContext, ConnState: observer.ConnState}
		if secure {
			cert, err := tls.LoadX509KeyPair(config.TLSCertFile, config.TLSKeyFile)
			if err != nil {
				l.Close()
				return fmt.Errorf("public TLS: %w", err)
			}
			server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}}
		}
		listeners = append(listeners, l)
		servers = append(servers, server)
		return nil
	}
	if err := add(config.Address, false); err != nil {
		return err
	}
	if err := add(config.TLSAddress, true); err != nil {
		return err
	}
	results := make(chan error, len(servers))
	for i, s := range servers {
		l := listeners[i]
		secure := s.TLSConfig != nil
		log.Printf("public HTTP listening on %s; TLS=%v", l.Addr(), secure)
		go func(s *http.Server, l net.Listener, secure bool) {
			if secure {
				results <- s.ServeTLS(l, "", "")
			} else {
				results <- s.Serve(l)
			}
		}(s, l, secure)
	}
	health := time.NewTicker(time.Second)
	defer health.Stop()
	var serveErr error
running:
	for {
		select {
		case <-ctx.Done():
			break running
		case err := <-results:
			if !errors.Is(err, http.ErrServerClosed) {
				serveErr = fmt.Errorf("public serve: %w", err)
			}
			break running
		case <-health.C:
			if err := logger.Err(); err != nil {
				serveErr = err
				break running
			}
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, s := range servers {
		if err := s.Shutdown(shutdown); err != nil {
			s.Close()
			serveErr = errors.Join(serveErr, err)
		}
	}
	return errors.Join(serveErr, logger.Close())
}
