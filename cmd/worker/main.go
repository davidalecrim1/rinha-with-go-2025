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
			MaxIdleConns:        30,
			MaxIdleConnsPerHost: 15,
			IdleConnTimeout:     120 * time.Second,
			MaxConnsPerHost:     20,
			DisableCompression:  true,
			DisableKeepAlives:   false,
			ForceAttemptHTTP2:   false,

			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
				KeepAlive: 30 * time.Second,
				DualStack: true,
			}).DialContext,
		},
	}

	redisClient := redis.NewClient(&redis.Options{
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
