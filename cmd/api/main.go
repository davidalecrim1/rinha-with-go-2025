package main

import (
	"net/http"
	"os"
	"time"

	"rinha-with-go-2025/internal"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
)

func main() {
	// TODO: Remove this after development.
	// slog.SetLogLoggerLevel(// slog.LevelDebug)

	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     120 * time.Second,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	}

	client := &http.Client{
		Transport: tr,
	}

	adapterDefaultUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")

	adapter := internal.NewPaymentProcessorAdapter(client, adapterDefaultUrl, adapterFallbackUrl)
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

	port := getEnvOrSetDefault("PORT", "9999")
	err := app.Listen(":" + port)
	if err != nil {
		panic(err)
	}
}

func getEnvOrSetDefault(key string, defaultVal string) string {
	// // slog.Debug(key)
	if os.Getenv(key) == "" {
		os.Setenv(key, defaultVal)
		return defaultVal
	}

	return os.Getenv(key)
}

func sonicMarshal(v interface{}) ([]byte, error) {
	return sonic.Marshal(v)
}

func sonicUnmarshal(data []byte, v interface{}) error {
	return sonic.Unmarshal(data, v)
}
