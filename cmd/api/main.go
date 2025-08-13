package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
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

	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			MaxConnsPerHost:     100,

			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,

			DisableCompression: true,
			DisableKeepAlives:  false,
			ForceAttemptHTTP2:  false,

			DialContext: (&net.Dialer{
				Timeout:   1 * time.Second,
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

	repo := internal.NewPaymentRepository(cfg)
	adapter := internal.NewPaymentAdapter(
		httpClient,
		redisClient,
		repo,
		cfg,
	)

	// unix socket client for the summary repository on the worker
	unixSocketClient := &fasthttp.HostClient{
		Addr: cfg.SummaryUnixSocketPath,
		Dial: func(addr string) (net.Conn, error) {
			return net.DialTimeout("unix", addr, 5*time.Second)
		},
	}
	handler := internal.NewPaymentHandler(adapter, unixSocketClient)

	// unix socker server for all handlers from the api
	if _, err := os.Stat(cfg.UnixSocketPath); err == nil {
		os.Remove(cfg.UnixSocketPath)
	}

	listener, err := net.Listen("unix", cfg.UnixSocketPath)
	if err != nil {
		panic(fmt.Errorf("error listening on unix socket: %v", err))
	}
	defer listener.Close()

	os.Chmod(cfg.UnixSocketPath, 0o666)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	slog.Info("starting app on unix socket", "socketPath", cfg.UnixSocketPath)
	if err := fasthttp.Serve(listener, handler.Router); err != nil {
		panic(fmt.Errorf("error starting fasthttp server: %v", err))
	}
}
