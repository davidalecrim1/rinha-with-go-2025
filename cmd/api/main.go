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
	"rinha-with-go-2025/pkg/utils"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shouldProfile := utils.GetEnvOrSetDefault("ENABLE_PROFILING", "false")
	if shouldProfile == "true" {
		profiling.EnableProfiling(time.Minute * 2)
	}

	slog.SetLogLoggerLevel(slog.LevelInfo)

	httpClient := &http.Client{}

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
	defaultUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	fallbackUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")
	adapter := internal.NewPaymentAdapter(
		httpClient,
		redisClient,
		repo,
		defaultUrl,
		fallbackUrl,
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

	go func() {
		<-ctx.Done()
		app.Shutdown()
	}()

	socketPath := utils.GetEnvOrSetDefault("UNIX_SOCKET", "/var/run/api.sock")
	if _, err := os.Stat(socketPath); err == nil {
		os.Remove(socketPath)
	}

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		panic(fmt.Errorf("error listening on unix socket: %v", err))
	}
	defer listener.Close()

	os.Chmod(socketPath, 0666)

	slog.Info("starting app on unix socket", "socketPath", socketPath)
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
