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
	"strconv"
	"time"

	"rinha-with-go-2025/internal"
	"rinha-with-go-2025/pkg/utils"

	"github.com/bytedance/sonic"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelInfo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	shutdown, tp, err := internal.NewTracer(ctx, os.Getenv("OTEL_SERVICE_NAME"))
	if err != nil {
		panic(fmt.Errorf("failed to initialize tracer: %v", err))
	}
	defer shutdown(ctx)

	redisAddr := utils.GetEnvOrSetDefault("REDIS_ADDR", "localhost:6379")
	redisClient := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		panic(fmt.Errorf("failed to connect to redis: %v", err))
	}
	repo := internal.NewPaymentRepository(tp.Tracer(), redisClient)

	workers := utils.GetEnvOrSetDefault("WORKERS", "20")
	workersInt, err := strconv.Atoi(workers)
	if err != nil {
		panic(fmt.Errorf("failed to convert workers to int: %v", err))
	}
	retryQueueSize := utils.GetEnvOrSetDefault("RETRY_QUEUE_SIZE", "6000")
	retryQueueSizeInt, err := strconv.Atoi(retryQueueSize)
	if err != nil {
		panic(fmt.Errorf("failed to convert retry queue size to int: %v", err))
	}
	retryQueue := make(chan internal.PaymentRequestProcessor, retryQueueSizeInt)

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
		Transport: otelhttp.NewTransport(tr),
	}

	adapterDefaultUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")
	adapter := internal.NewPaymentProcessorAdapter(
		tp.Tracer(),
		httpClient,
		redisClient,
		repo,
		adapterDefaultUrl,
		adapterFallbackUrl,
		retryQueue,
		workersInt,
	)

	handler := internal.NewPaymentHandler(adapter, tp.Tracer())
	app := fiber.New(fiber.Config{
		JSONEncoder: sonicMarshal,
		JSONDecoder: sonicUnmarshal,

		Prefork:       false,
		CaseSensitive: true,
		StrictRouting: false,
		ServerHeader:  "Fiber",
		AppName:       "High Performance API",
	})
	app.Use(otelfiber.Middleware())
	handler.RegisterRoutes(app)

	shouldMonitorHealth := utils.GetEnvOrSetDefault("MONITOR_HEALTH", "true")
	if shouldMonitorHealth == "true" {
		adapter.EnableHealthCheck()
	}

	shouldProfile := utils.GetEnvOrSetDefault("ENABLE_PROFILING", "false")
	if shouldProfile == "true" {
		enableProfiling()
	}

	adapter.StartWorkers()

	port := utils.GetEnvOrSetDefault("PORT", "9999")
	err = app.Listen(":" + port)
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

func enableProfiling() {
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
