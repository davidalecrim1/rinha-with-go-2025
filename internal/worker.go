package internal

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
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
	HealthCheckTicker         = 5 * time.Second
	BackoffTimeEmptyQueue     = 1 * time.Second
	BackoffTimeAfterRequest   = 1 * time.Millisecond
	MinAcceptableResponseTime = 200 // in milliseconds
)

type PaymentProcessor struct {
	redis                *redis.Client
	client               *http.Client
	repo                 *PaymentRepository
	healthStatusDefault  atomic.Value
	healthStatusFallback atomic.Value
	defaultUrl           string
	fallbackUrl          string
	workers              int
}

func NewPaymentProcessor(redis *redis.Client, client *http.Client, repo *PaymentRepository, defaultUrl, fallbackUrl string, workers int) *PaymentProcessor {
	w := &PaymentProcessor{
		redis:       redis,
		client:      client,
		repo:        repo,
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,
		workers:     workers,
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
			w.defaultUrl+"/payments",
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
	start1 := time.Now()
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

	res, err := w.client.Do(req)
	slog.Debug("response from the processor", "res", res, "err", err)
	if res != nil {
		defer res.Body.Close()
	}
	if res != nil && res.StatusCode == 422 {
		return nil
	}
	if res != nil && res.StatusCode == 500 {
		return ErrUnavailableProcessor
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrUnavailableProcessor
	}

	if res != nil && res.StatusCode != 200 {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}
	if err != nil || res == nil {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}

	start2 := time.Now()
	err = w.repo.Add(PaymentProcessed{
		PaymentRequestProcessor: payment,
		Processed:               endpoint,
	})

	if time.Since(start1).Milliseconds() > 80 {
		slog.Debug("time of the complete request and db",
			"dbTimeMs", time.Since(start2).Milliseconds(),
			"requestTimeMs", time.Since(start1).Milliseconds(),
			"healthStatusDefault", w.healthStatusDefault.Load().(HealthCheckResponse),
			"healthStatusFallback", w.healthStatusFallback.Load().(HealthCheckResponse),
			"endpoint", endpoint,
			"err", err,
			"requestAt", *payment.RequestedAt,
		)
	}
	return err
}

func (w *PaymentProcessor) EnableHealthCheck(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := w.storeHealthStatus(w.defaultUrl+"/payments/service-health", HealthCheckKeyDefault); err != nil {
					slog.Debug("failed to update the health check", "err", err)
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
				if err := w.storeHealthStatus(w.fallbackUrl+"/payments/service-health", HealthCheckKeyFallback); err != nil {
					slog.Debug("failed to update the health check", "err", err)
				}
			}
		}
	}()
}

func (w *PaymentProcessor) storeHealthStatus(url string, key string) error {
	resDefault, err := w.retrieveHealth(url)
	if err != nil {
		return err
	}

	reqbody := HealthCheckResponse{
		Failing:         resDefault.Failing,
		MinResponseTime: resDefault.MinResponseTime,
	}
	rawBody, err := sonic.ConfigFastest.Marshal(reqbody)
	if err != nil {
		slog.Debug("failed to encode the json object for redis", "err", err)
		return err
	}

	if err := w.redis.Set(context.Background(), key, rawBody, 0).Err(); err != nil {
		slog.Debug("failed to save health check in redis", "err", err)
		return err
	}

	slog.Debug("updating the health check", "healthCheckStatus", reqbody, "key", key)
	return nil
}

func (w *PaymentProcessor) retrieveHealth(url string) (HealthCheckResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return HealthCheckResponse{}, err
	}

	res, err := w.client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}
	if res == nil || err != nil || res.StatusCode != 200 {
		slog.Debug("failed to health check", "url", url)
		return HealthCheckResponse{}, err
	}

	var respBody HealthCheckResponse
	decoder := sonic.ConfigFastest.NewDecoder(res.Body)
	if err := decoder.Decode(&respBody); err != nil {
		slog.Debug("failed to parse the response", "url", url)
		return HealthCheckResponse{}, err
	}

	return respBody, nil
}

func (w *PaymentProcessor) StartWorkers(ctx context.Context) {
	for range w.workers {
		go w.run(ctx)
	}

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := w.syncHealthStatus(HealthCheckKeyDefault); err != nil {
				slog.Debug("failed update the health check", "err", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := w.syncHealthStatus(HealthCheckKeyFallback); err != nil {
				slog.Debug("failed update the health check", "err", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			len, err := w.redis.LLen(context.Background(), PaymentProcessingQueue).Result()
			if err != nil {
				slog.Error("failed to get the length of the queue", "err", err)
				continue
			}

			slog.Info("length of the queue", "length", len)
		}
	}()
}

func (w *PaymentProcessor) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			raw, err := w.redis.LPop(ctx, PaymentProcessingQueue).Bytes()
			if err != nil {
				slog.Debug("failed to pop the payment from the queue", "error", err)
				time.Sleep(BackoffTimeEmptyQueue) // add delay when queue is empty
				continue
			}

			var payment PaymentRequestProcessor
			if err := sonic.ConfigFastest.Unmarshal(raw, &payment); err != nil {
				slog.Info("failed to unmarshal the payment", "error", err, "raw", string(raw))
				continue
			}

			//time.Sleep(BackoffTimeAfterRequest) // backoff to avoid network overload

			if err := w.innerProcess(payment); err != nil {
				w.redis.LPush(ctx, PaymentProcessingQueue, raw)
			}
		}
	}
}

func (w *PaymentProcessor) syncHealthStatus(key string) error {
	resBody, err := w.redis.Get(context.Background(), key).Result()
	if err != nil {
		slog.Debug("failed to get the health check", "err", err)
		return err
	}

	var healthCheckStatus HealthCheckResponse
	if err := sonic.ConfigFastest.Unmarshal([]byte(resBody), &healthCheckStatus); err != nil {
		slog.Debug("failed to unmarshal the health check from redis", "err", err)
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
