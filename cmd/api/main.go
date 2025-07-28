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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// TODO: Remove this after development.
	slog.SetLogLoggerLevel(slog.LevelInfo)
	ctx := context.Background()

	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     100,

		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		DisableCompression: true,
		DisableKeepAlives:  false,
		ForceAttemptHTTP2:  false,

		DialContext: (&net.Dialer{
			Timeout:   100 * time.Millisecond,
			KeepAlive: 90 * time.Second,
			DualStack: true,
		}).DialContext,
	}
	client := &http.Client{
		Transport: tr,
	}

	endpoint := utils.GetEnvOrSetDefault("MONGO_ENDPOINT", "localhost:27017")
	if endpoint == "" {
		panic("the mongo db endpoint is not provided")
	}

	opts := options.
		Client().
		ApplyURI(endpoint).
		SetServerSelectionTimeout(time.Second * 5).
		SetMaxConnIdleTime(30 * time.Second).
		SetMinPoolSize(40).
		SetMaxPoolSize(100)

	mdbClient, err := mongo.Connect(ctx, opts)
	if err != nil {
		panic(fmt.Sprintf("failed to connect to mongodb: %v", err))
	}
	err = mdbClient.Ping(ctx, nil)
	if err != nil {
		panic(fmt.Sprintf("failed to ping mongodb: %v", err))
	}
	mdb := mdbClient.Database(utils.GetEnvOrSetDefault("MONGO_DATABASE", "payments-db"))

	repo := internal.NewPaymentRepository(mdb)
	workers := 300
	slowQueue := make(chan internal.PaymentRequestProcessor, 3000)

	redisAddr := utils.GetEnvOrSetDefault("REDIS_ADDR", "localhost:6379")
	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		panic(fmt.Errorf("failed to connect to redis: %v", err))
	}

	adapterDefaultUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "http://localhost:8001")
	adapterFallbackUrl := utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "http://localhost:8002")
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
