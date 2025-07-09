package main

import (
	"net/http"
	"os"
	"time"

	"rinha-with-go-2025/internal"

	"github.com/gofiber/fiber/v2"
	"resty.dev/v3"
)

func main() {
	// TODO: Remove this after development.
	// slog.SetLogLoggerLevel(// slog.LevelDebug)

	tr := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     120 * time.Second,
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
	}

	client := resty.New().
		SetTransport(tr).
		SetRetryCount(0) // I won't be using the retry from Resty.
		// AddResponseMiddleware(func(c *resty.Client, resp *resty.Response) error {
		// 	slog.Debug("Request completed",
		// 		"method", resp.Request.Method,
		// 		"url", resp.Request.URL,
		// 		"status", resp.StatusCode(),
		// 		"body", resp.String(),
		// 	)
		// 	return nil
		// }).
		// OnError(func(req *resty.Request, err error) {
		// 	if v, ok := err.(*resty.ResponseError); ok {
		// 		slog.Error("request failed after retries",
		// 			"method", req.Method,
		// 			"url", req.URL,
		// 			"status", v.Response.StatusCode(),
		// 			"body", v.Response.String(),
		// 			"attempts", req.Attempt,
		// 		)
		// 	} else {
		// 		slog.Error("request failed with non-retryable error",
		// 			"method", req.Method,
		// 			"url", req.URL,
		// 			"error", err.Error(),
		// 		)
		// 	}
		// })

	adapterDefaultUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := getEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")

	adapter := internal.NewPaymentProcessorAdapter(client, adapterDefaultUrl, adapterFallbackUrl)
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
	// // slog.Debug(key)
	if os.Getenv(key) == "" {
		os.Setenv(key, defaultVal)
		return defaultVal
	}

	return os.Getenv(key)
}
