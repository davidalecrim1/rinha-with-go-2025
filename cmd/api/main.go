package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"rinha-with-go-2025/internal"
	"rinha-with-go-2025/pkg/utils"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

func main() {
	// TODO: Remove this after development.
	slog.SetLogLoggerLevel(slog.LevelInfo)

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
	redisAddr := utils.GetEnvOrSetDefault("REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic("failed to connect to redis")
	}

	repo := internal.NewPaymentRepository(rdb)
	adapterDefaultUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")
	workers := 300
	slowQueue := make(chan internal.PaymentRequestProcessor, 3000)

	adapter := internal.NewPaymentProcessorAdapter(
		client,
		rdb,
		repo,
		adapterDefaultUrl,
		adapterFallbackUrl,
		slowQueue,
		workers,
	)

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

	shouldMonitorHealth := utils.GetEnvOrSetDefault("MONITOR_HEALTH", "true")
	adapter.EnableHealthCheck(shouldMonitorHealth)

	shouldProfile := utils.GetEnvOrSetDefault("ENABLE_PROFILING", "false")
	enableProfiling(shouldProfile)

	adapter.StartWorkers()

	port := utils.GetEnvOrSetDefault("PORT", "9999")
	err := app.Listen(":" + port)
	if err != nil {
		panic(fmt.Errorf("failed to listen to port: %v", err))
	}
}

func sonicMarshal(v any) ([]byte, error) {
	return sonic.Marshal(v)
}

func sonicUnmarshal(data []byte, v any) error {
	return sonic.Unmarshal(data, v)
}

func enableProfiling(shouldProfile string) {
	if shouldProfile != "true" {
		return
	}

	slog.Info("profiling enabled")

	err := os.Mkdir("prof", 0o755)
	if err != nil {
		slog.Error("failed to create profiling directory", "err", err)
	}

	cf, err := os.Create("./prof/cpu.prof")
	if err != nil {
		slog.Error("failed to start CPU profiling", "error", err)
	}
	pprof.StartCPUProfile(cf)

	mf, err := os.Create("./prof/memory.prof")
	if err != nil {
		slog.Error("failed to start memory profiling", "error", err)
	}
	pprof.WriteHeapProfile(mf)

	tc, err := os.Create("./prof/trace.prof")
	if err != nil {
		slog.Error("failed to start trace profiling", "error", err)
	}
	trace.Start(tc)

	stop := time.After(time.Minute * 2)

	go func() {
		<-stop
		pprof.StopCPUProfile()
		trace.Stop()
		cf.Close()
		mf.Close()
		tc.Close()
		slog.Info("finished the profiling")
	}()
}
