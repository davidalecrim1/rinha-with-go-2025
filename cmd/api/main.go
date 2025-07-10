package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"rinha-with-go-2025/internal"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func main() {
	// TODO: Remove this after development.
	slog.SetLogLoggerLevel(slog.LevelDebug)

	tr := &http.Transport{
		MaxIdleConns:        30,
		MaxIdleConnsPerHost: 15,
		IdleConnTimeout:     120 * time.Second,
		MaxConnsPerHost:     20,
		DisableCompression:  true,
		DisableKeepAlives:   false,
		ForceAttemptHTTP2:   false,

		DialContext: (&net.Dialer{
			Timeout:   1 * time.Second,  // Fast connection establishment
			KeepAlive: 30 * time.Second, // Keep TCP connections alive
			DualStack: true,             // Enable IPv4/IPv6
		}).DialContext,
	}

	client := &http.Client{
		Transport: tr,
	}

	adapterDefaultUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")

	workers := 5000
	slowQueue := make(chan internal.PaymentRequestProcessor, 5000)

	redisAddr := getEnvOrSetDefault("REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic("failed to connect to redis")
	}

	adapter := internal.NewPaymentProcessorAdapter(client, rdb, adapterDefaultUrl, adapterFallbackUrl, slowQueue, workers)
	handler := internal.NewPaymentHandler(adapter)

	app := fiber.New(fiber.Config{
		JSONEncoder: sonicMarshal,
		JSONDecoder: sonicUnmarshal,

		Prefork:       false,
		CaseSensitive: true,
		StrictRouting: false,
		ServerHeader:  "Fiber",
		AppName:       "High Performance API",
	})

	app.Post("/payments", handler.Process)
	app.Get("/payments-summary", handler.Summary)
	app.Post("/purge-payments", handler.Purge)

	adapter.StartWorkers()
	monitorHealth := getEnvOrSetDefault("MONITOR_HEALTH", "true")
	adapter.EnableHealthCheck(monitorHealth)

	port := getEnvOrSetDefault("PORT", "9999")
	err := app.Listen(":" + port)
	if err != nil {
		panic(fmt.Errorf("failed to listen to port: %v", err))
	}
}

func getEnvOrSetDefault(key string, defaultVal string) string {
	if os.Getenv(key) == "" {
		os.Setenv(key, defaultVal)
		return defaultVal
	}

	return os.Getenv(key)
}

func sonicMarshal(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

func sonicUnmarshal(data []byte, v any) error {
	return sonic.Unmarshal(data, v)
}
