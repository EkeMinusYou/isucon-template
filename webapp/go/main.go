package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config := serverConfig{
		Address: envOr("APP_ADDR", ":80"), TLSAddress: os.Getenv("TLS_ADDR"),
		TLSCertFile: os.Getenv("TLS_CERT_FILE"), TLSKeyFile: os.Getenv("TLS_KEY_FILE"),
		MeasurementAddr: envOr("MEASUREMENT_ADDR", "127.0.0.1:6060"),
		AccessLog:       envOr("ACCESS_LOG", "/home/isucon/log/access.log"),
		StaticDir:       envOr("STATIC_DIR", "../../public"), StaticPrefix: envOr("STATIC_URL_PREFIX", "/static/"),
	}
	if err := serve(ctx, config, newHandler()); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
