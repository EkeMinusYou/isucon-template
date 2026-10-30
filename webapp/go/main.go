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
		Address:         envOr("APP_ADDR", ":8080"),
		MeasurementAddr: envOr("MEASUREMENT_ADDR", "127.0.0.1:6060"),
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
