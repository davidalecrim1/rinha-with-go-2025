package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"rinha-with-go-2025/internal"

	"github.com/gofiber/fiber/v2"
	"resty.dev/v3"
)

func main() {
	// TODO: Remove this after development.
	slog.SetLogLoggerLevel(slog.LevelDebug)

	tr := &http.Transport{
		MaxIdleConns:    0,
		IdleConnTimeout: 90 * time.Second,
	}

	client := resty.New().
		SetTransport(tr).
		SetAllowNonIdempotentRetry(true).
		AddRetryConditions(
			func(r *resty.Response, err error) bool {
				if err != nil {
					return true
				}

				return r.StatusCode() >= 500
			},
		// )
		).
		AddResponseMiddleware(func(c *resty.Client, resp *resty.Response) error {
			slog.Info("Request completed",
				"method", resp.Request.Method,
				"url", resp.Request.URL,
				"status", resp.StatusCode(),
				"body", resp.String(),
			)
			return nil
		}).
		OnError(func(req *resty.Request, err error) {
			if v, ok := err.(*resty.ResponseError); ok {
				slog.Error("request failed after retries",
					"method", req.Method,
					"url", req.URL,
					"status", v.Response.StatusCode(),
					"body", v.Response.String(),
					"attempts", req.Attempt,
				)
			} else {
				slog.Error("request failed with non-retryable error",
					"method", req.Method,
					"url", req.URL,
					"error", err.Error(),
				)
			}
		})

	adapterDefaultUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")

	adapter := internal.NewPaymentProcessorAdapter(client, adapterDefaultUrl, adapterFallbackUrl)
	adapter.StartWorkers()
	handler := internal.NewPaymentHandler(adapter)

	app := fiber.New()
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
	slog.Debug(key)
	if os.Getenv(key) == "" {
		os.Setenv(key, defaultVal)
		return defaultVal
	}

	return os.Getenv(key)
}
