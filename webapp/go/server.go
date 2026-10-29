package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	measurement "github.com/EkeMinusYou/isucon-template/tools/measurement/app"
)

type serverConfig struct {
	Address         string                      `json:"address"`
	MeasurementAddr string                      `json:"measurement_address"`
	AccessLogPath   string                      `json:"access_log_path"`
	TLSCertFile     string                      `json:"tls_cert_file"`
	TLSKeyFile      string                      `json:"tls_key_file"`
	MaxConnections  int                         `json:"max_connections"`
	StaticRoutes    []staticRoute               `json:"static_routes"`
	Snapshot        func() measurement.Snapshot `json:"-"`
}

type limitedListener struct {
	net.Listener
	slots  chan struct{}
	done   chan struct{}
	once   sync.Once
	active atomic.Int64
}
type limitedConn struct {
	net.Conn
	owner *limitedListener
	once  sync.Once
}

func (c *limitedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { <-c.owner.slots; c.owner.active.Add(-1) })
	return err
}
func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	c, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	l.active.Add(1)
	return &limitedConn{Conn: c, owner: l}, nil
}
func (l *limitedListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

func serve(ctx context.Context, config serverConfig, handler http.Handler) error {
	host, _, err := net.SplitHostPort(config.MeasurementAddr)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("MEASUREMENT_ADDR must bind a loopback IP: %q", config.MeasurementAddr)
	}
	if (config.TLSCertFile == "") != (config.TLSKeyFile == "") {
		return fmt.Errorf("both TLS certificate and key are required")
	}
	handler, err = staticHandler(config.StaticRoutes, handler)
	if err != nil {
		return err
	}
	public := handler
	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/debug" || len(r.URL.Path) >= 7 && r.URL.Path[:7] == "/debug/" {
			http.NotFound(w, r)
			return
		}
		public.ServeHTTP(w, r)
	})
	if config.AccessLogPath == "" {
		config.AccessLogPath = "access.log"
	}
	file, err := os.OpenFile(config.AccessLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	defer file.Close()
	logger, err := measurement.NewAccessLogger(file, measurement.AccessLoggerOptions{})
	if err != nil {
		return err
	}
	defer logger.Close()
	lc := net.ListenConfig{KeepAlive: 3 * time.Minute}
	raw, err := lc.Listen(ctx, "tcp", config.Address)
	if err != nil {
		return fmt.Errorf("application listen: %w", err)
	}
	if config.MaxConnections < 1 {
		config.MaxConnections = 32768
	}
	listener := &limitedListener{Listener: raw, slots: make(chan struct{}, config.MaxConnections), done: make(chan struct{})}
	defer listener.Close()
	metrics, err := measurement.Start(measurement.Options{Address: config.MeasurementAddr, Snapshot: func() measurement.Snapshot {
		snapshot := measurement.Snapshot{}
		if config.Snapshot != nil {
			snapshot = config.Snapshot()
		}
		values := make(map[string]any, len(snapshot.Metrics)+6)
		for k, v := range snapshot.Metrics {
			values[k] = v
		}
		snapshot.Metrics = values
		for k, v := range logger.Metrics() {
			snapshot.Metrics[k] = v
		}
		snapshot.Metrics["http_connections_active"] = listener.active.Load()
		snapshot.Metrics["http_connections_limit"] = config.MaxConnections
		return snapshot
	}})
	if err != nil {
		return err
	}
	defer metrics.Close()
	var nextID atomic.Uint64
	nextID.Store(uint64(time.Now().UnixMilli()) << 20)
	server := &http.Server{Handler: measuredHandler(handler, logger), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 120 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, connectionKey{}, &connectionObservation{id: nextID.Add(1)})
	}}
	defer server.Close()
	result := make(chan error, 1)
	go func() {
		if config.TLSCertFile != "" {
			result <- server.ServeTLS(listener, config.TLSCertFile, config.TLSKeyFile)
		} else {
			result <- server.Serve(listener)
		}
	}()
	log.Printf("application listening on %s; measurements on %s", listener.Addr(), metrics.Addr)
	select {
	case err = <-result:
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return logger.Close()
}
