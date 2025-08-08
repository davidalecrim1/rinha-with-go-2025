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

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
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

	tr := &http.Transport{
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
	}

	httpClient := &http.Client{
		Transport: tr,
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
	adapter := internal.NewPaymentAdapter(
		httpClient,
		redisClient,
		repo,
		cfg,
	)

	handler := internal.NewPaymentHandler(adapter)
	app := fiber.New(fiber.Config{
		JSONEncoder: sonicMarshal,
		JSONDecoder: sonicUnmarshal,

		Prefork:                   false,
		CaseSensitive:             true,
		StrictRouting:             false,
		ServerHeader:              "",
		AppName:                   "",
		DisableDefaultDate:        true,
		DisableDefaultContentType: true,
		DisableHeaderNormalizing:  true,
		DisableStartupMessage:     true,
	})

	handler.RegisterRoutes(app)

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
		app.Shutdown()
	}()

	slog.Info("starting app on unix socket", "socketPath", cfg.UnixSocketPath)
	if err := app.Listener(listener); err != nil {
		panic(fmt.Errorf("error starting Fiber app: %v", err))
	}
}

func sonicMarshal(v any) ([]byte, error) {
	return sonic.ConfigFastest.Marshal(v)
}

func sonicUnmarshal(data []byte, v any) error {
	return sonic.ConfigFastest.Unmarshal(data, v)
}
