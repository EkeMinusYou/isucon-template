package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	config, err := configFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serve(ctx, config, newHandler()); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func configFromEnv() (serverConfig, error) {
	http2Enabled, err := strconv.ParseBool(envOr("HTTP2_ENABLED", "true"))
	if err != nil {
		return serverConfig{}, fmt.Errorf("HTTP2_ENABLED must be a boolean: %w", err)
	}
	return serverConfig{
		Address: envOr("APP_ADDR", ":80"), TLSAddress: os.Getenv("TLS_ADDR"),
		TLSCertFile: os.Getenv("TLS_CERT_FILE"), TLSKeyFile: os.Getenv("TLS_KEY_FILE"),
		HTTP2Enabled:    &http2Enabled,
		MeasurementAddr: envOr("MEASUREMENT_ADDR", "127.0.0.1:6060"),
		AccessLog:       envOr("ACCESS_LOG", "/home/isucon/log/access.log"),
		StaticDir:       envOr("STATIC_DIR", "../../public"), StaticPrefix: envOr("STATIC_URL_PREFIX", "/static/"),
	}, nil
}
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
