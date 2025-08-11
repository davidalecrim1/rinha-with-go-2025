package internal

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
)

const (
	PaymentProcessingQueue = "queue:payments"
)

const (
	HealthCheckKeyDefault     = "health-check:default"
	HealthCheckKeyFallback    = "health-check:fallback"
	HealthCheckTicker         = 1 * time.Second
	BackoffTimeEmptyQueue     = 100 * time.Millisecond
	MinAcceptableResponseTime = 200 // in milliseconds
	MinJitterBetweenPayments  = 10  // in milliseconds
	MaxJitterBetweenPayments  = 20  // in milliseconds
)

type PaymentProcessor struct {
	redisClient          *redis.Client
	httpClient           *http.Client
	repo                 *PaymentRepository
	healthStatusDefault  atomic.Value
	healthStatusFallback atomic.Value
	cfg                  *Config
}

func NewPaymentProcessor(redis *redis.Client, httpClient *http.Client, repo *PaymentRepository, cfg *Config) *PaymentProcessor {
	w := &PaymentProcessor{
		redisClient: redis,
		httpClient:  httpClient,
		repo:        repo,
		cfg:         cfg,
	}

	w.healthStatusDefault.Store(HealthCheckResponse{
		Failing:         false,
		MinResponseTime: 0,
	})

	w.healthStatusFallback.Store(HealthCheckResponse{
		Failing:         false,
		MinResponseTime: 0,
	})

	return w
}

func (w *PaymentProcessor) innerProcess(payment PaymentRequestProcessor) error {
	healthStatusDefault := w.healthStatusDefault.Load().(HealthCheckResponse)

	var err error
	if !healthStatusDefault.Failing && healthStatusDefault.MinResponseTime < MinAcceptableResponseTime {
		err = w.sendPayment(
			payment,
			w.cfg.PaymentProcessorDefault+"/payments",
			time.Second*10,
			PaymentEndpointDefault,
		)
	} else {
		return ErrUnavailableProcessor
	}

	if errors.Is(err, ErrInvalidRequest) {
		return nil
	}

	return err
}

func (w *PaymentProcessor) sendPayment(
	payment PaymentRequestProcessor,
	url string,
	timeout time.Duration,
	endpoint PaymentEndpoint,
) error {
	payment.UpdateRequestTime()
	raw, err := sonic.ConfigFastest.Marshal(payment)
	if err != nil {
		slog.Error("failed to marshal the payment", "err", err)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		slog.Error("failed to create the request", "err", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	res, err := w.httpClient.Do(req)
	// slog.Debug("response from the processor", "res", res, "err", err)
	if err != nil || res == nil {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}
	if res.StatusCode == 422 {
		return nil
	}
	if res.StatusCode == 500 {
		return ErrUnavailableProcessor
	}
	if res.StatusCode != 200 {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}

	return w.repo.Add(PaymentProcessed{
		PaymentRequestProcessor: payment,
		Processed:               endpoint,
	})
}

func (w *PaymentProcessor) EnableHealthCheck(ctx context.Context) {
	// Retrieve the health check from the processors and store in redis
	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.storeHealthStatus("/payments/service-health", HealthCheckKeyDefault); err != nil {
					// slog.Debug("failed to update the health check", "err", err)
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.storeHealthStatus("/payments/service-health", HealthCheckKeyFallback); err != nil {
					// slog.Debug("failed to update the health check", "err", err)
				}
			}
		}
	}()
}

func (w *PaymentProcessor) storeHealthStatus(path string, key string) error {
	resDefault, err := w.retrieveHealth(path)
	if err != nil {
		return err
	}

	reqbody := HealthCheckResponse{
		Failing:         resDefault.Failing,
		MinResponseTime: resDefault.MinResponseTime,
	}
	rawBody, err := sonic.ConfigFastest.Marshal(reqbody)
	if err != nil {
		// slog.Debug("failed to encode the json object for redis", "err", err)
		return err
	}

	if err := w.redisClient.Set(context.Background(), key, rawBody, 0).Err(); err != nil {
		// slog.Debug("failed to save health check in redis", "err", err)
		return err
	}

	// slog.Debug("updating the health check", "healthCheckStatus", reqbody, "key", key)
	return nil
}

func (w *PaymentProcessor) retrieveHealth(path string) (HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		// slog.Debug("failed to health check", "path", path)
		return HealthCheckResponse{}, err
	}

	res, err := w.httpClient.Do(req)
	if err != nil {
		// slog.Debug("failed to health check", "path", path)
		return HealthCheckResponse{}, err
	}
	defer res.Body.Close()

	var body HealthCheckResponse
	if err := sonic.ConfigFastest.NewDecoder(res.Body).Decode(&body); err != nil {
		// slog.Debug("failed to parse the response", "path", path)
		return HealthCheckResponse{}, err
	}

	return body, nil
}

func (w *PaymentProcessor) StartWorkers(ctx context.Context) {
	for range w.cfg.NumWorkers {
		go w.run(ctx)
	}

	// Pull from redis the latest health check status
	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := w.syncHealthStatus(HealthCheckKeyDefault); err != nil {
				// slog.Debug("failed update the health check", "err", err)
			}
		}
	}()

	// Pull from redis the latest health check status
	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := w.syncHealthStatus(HealthCheckKeyFallback); err != nil {
				// slog.Debug("failed update the health check", "err", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			redisQueueLength, err := w.redisClient.LLen(context.Background(), PaymentProcessingQueue).Result()
			if err != nil {
				slog.Error("failed to get the length of the queue", "err", err)
				continue
			}

			slog.Info("length of the queue", "redisQueueLength", redisQueueLength)
		}
	}()
}

func (w *PaymentProcessor) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			raw, err := w.redisClient.LPop(ctx, PaymentProcessingQueue).Bytes()
			if err != nil {
				slog.Debug("failed to pop the payment from the queue", "error", err)
				time.Sleep(BackoffTimeEmptyQueue) // add delay when queue is empty
				continue
			}

			time.Sleep(time.Millisecond * time.Duration(rand.Intn(MaxJitterBetweenPayments-MinJitterBetweenPayments)+MinJitterBetweenPayments))

			var payment PaymentRequestProcessor
			if err := sonic.ConfigFastest.Unmarshal(raw, &payment); err != nil {
				slog.Info("failed to unmarshal the payment", "error", err, "raw", string(raw))
				continue
			}

			if err := w.innerProcess(payment); err != nil {
				w.redisClient.LPush(ctx, PaymentProcessingQueue, raw)
			}
		}
	}
}

func (w *PaymentProcessor) syncHealthStatus(key string) error {
	resBody, err := w.redisClient.Get(context.Background(), key).Result()
	if err != nil {
		// slog.Debug("failed to get the health check", "err", err)
		return err
	}

	var healthCheckStatus HealthCheckResponse
	if err := sonic.ConfigFastest.Unmarshal([]byte(resBody), &healthCheckStatus); err != nil {
		// slog.Debug("failed to unmarshal the health check from redis", "err", err)
		return err
	}

	switch key {
	case HealthCheckKeyDefault:
		w.healthStatusDefault.Store(healthCheckStatus)
	case HealthCheckKeyFallback:
		w.healthStatusFallback.Store(healthCheckStatus)
	}

	return nil
}
