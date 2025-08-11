package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"rinha-with-go-2025/internal"
	"rinha-with-go-2025/pkg/profiling"

	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := internal.NewConfig()
	cfg.Load()

	slog.SetLogLoggerLevel(cfg.LogLevel)

	if cfg.EnableProfiling {
		profiling.ProfileApplication(time.Minute * 2)
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,

			IdleConnTimeout:       60 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,

			DisableCompression: true,
			DisableKeepAlives:  false,
			ForceAttemptHTTP2:  false,

			DialContext: (&net.Dialer{
				KeepAlive: 90 * time.Second,
				DualStack: true,
			}).DialContext,
		},
	}

	redisClient := redis.NewClient(&redis.Options{
		Network:  "unix",
		Addr:     cfg.RedisAddr,
		Password: "",
		DB:       0,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		panic(fmt.Errorf("failed to connect to redis: %w", err))
	}

	repo := internal.NewPaymentRepository(redisClient)
	processor := internal.NewPaymentProcessor(
		redisClient,
		httpClient,
		repo,
		cfg,
	)

	if cfg.MonitorHealthPaymentProcessors {
		processor.EnableHealthCheck(ctx)
	}

	processor.StartWorkers(ctx)

	<-ctx.Done()
	slog.Info("shutting down workers...")
}
