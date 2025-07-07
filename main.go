package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"resty.dev/v3"
)

func main() {
	// TODO: Remove this after development.
	slog.SetLogLoggerLevel(slog.LevelDebug)

	tr := &http.Transport{
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}

	client := resty.New().
		SetTimeout(5 * time.Second).
		SetTransport(tr).
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetAllowNonIdempotentRetry(true).
		AddRetryConditions(
			func(r *resty.Response, err error) bool {
				return r.StatusCode() >= 500
			},
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
				)
			} else {
				slog.Error("request failed with non-retryable error",
					"method", req.Method,
					"url", req.URL,
					"error", err.Error(),
				)
			}
		})

	adapterDefaultUrl := os.Getenv("PAYMENT_PROCESSOR_URL_DEFAULT")
	if adapterDefaultUrl == "" {
		adapterDefaultUrl = "http://localhost:8001"
		// adapterDefaultUrl = "http://payment-processor-default:8080"
	}
	adapterFallbackUrl := os.Getenv("PAYMENT_PROCESSOR_URL_FALLBACK")
	if adapterFallbackUrl == "" {
		adapterFallbackUrl = "http://localhost:8002"
		// adapterFallbackUrl = "http://payment-processor-fallback:8080"
	}

	adapter := &PaymentProcessorAdapter{client, adapterDefaultUrl, adapterFallbackUrl}
	handler := &PaymentHandler{adapter}

	app := fiber.New()
	app.Post("/payments", handler.Process)
	app.Get("/payments-summary", handler.Summary)

	err := app.Listen(":9999")
	if err != nil {
		panic(err)
	}
}
