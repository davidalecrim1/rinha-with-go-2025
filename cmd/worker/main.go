package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"rinha-with-go-2025/internal"
	"rinha-with-go-2025/pkg/profiling"

	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
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

	defaultFasthttpClient := &fasthttp.HostClient{
		Addr:                          cfg.PaymentProcessorDefault,
		MaxConns:                      300,
		ReadTimeout:                   10 * time.Second,
		WriteTimeout:                  10 * time.Second,
		MaxIdleConnDuration:           60 * time.Second,
		DisableHeaderNamesNormalizing: true,
		NoDefaultUserAgentHeader:      true,
	}

	fallbackFasthttpClient := &fasthttp.HostClient{
		Addr:                          cfg.PaymentProcessorFallback,
		MaxConns:                      300,
		ReadTimeout:                   10 * time.Second,
		WriteTimeout:                  10 * time.Second,
		MaxIdleConnDuration:           60 * time.Second,
		DisableHeaderNamesNormalizing: true,
		NoDefaultUserAgentHeader:      true,
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
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
		defaultFasthttpClient,
		fallbackFasthttpClient,
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
