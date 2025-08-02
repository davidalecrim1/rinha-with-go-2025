package main

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"rinha-with-go-2025/internal"
	"rinha-with-go-2025/pkg/profiling"
	"rinha-with-go-2025/pkg/utils"

	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shouldProfile := utils.GetEnvOrSetDefault("ENABLE_PROFILING", "false")
	if shouldProfile == "true" {
		profiling.EnableProfiling(time.Minute * 2)
	}

	slog.SetLogLoggerLevel(slog.LevelInfo)

	defaultUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "localhost:8001")
	_ = utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "localhost:8002")

	httpClient := &fasthttp.HostClient{
		Addr:                          defaultUrl,
		MaxConns:                      200,
		ReadTimeout:                   10 * time.Second,
		WriteTimeout:                  10 * time.Second,
		MaxIdleConnDuration:           60 * time.Second,
		DisableHeaderNamesNormalizing: true,
		NoDefaultUserAgentHeader:      true,
	}

	redisAddr := utils.GetEnvOrSetDefault("REDIS_ADDR", "localhost:6379")
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		panic(fmt.Errorf("failed to connect to redis: %w", err))
	}

	repo := internal.NewPaymentRepository(redisClient)
	workers := utils.GetEnvOrSetDefault("WORKERS", "5")
	workersInt, _ := strconv.Atoi(workers)

	processor := internal.NewPaymentProcessor(
		redisClient,
		httpClient,
		repo,
		workersInt,
	)

	shouldMonitorHealth := utils.GetEnvOrSetDefault("MONITOR_HEALTH", "true")
	if shouldMonitorHealth == "true" {
		processor.EnableHealthCheck(ctx)
	}

	processor.StartWorkers(ctx)

	<-ctx.Done()
	slog.Info("shutting down worker")
}
