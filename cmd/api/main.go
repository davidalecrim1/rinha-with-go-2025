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

	port := utils.GetEnvOrSetDefault("PORT", "9999")
	err := app.Listen(":" + port)
	if err != nil {
		panic(fmt.Errorf("failed to listen to port: %v", err))
	}
}

func sonicMarshal(v any) ([]byte, error) {
	return sonic.ConfigFastest.Marshal(v)
}

func sonicUnmarshal(data []byte, v any) error {
	return sonic.ConfigFastest.Unmarshal(data, v)
}
