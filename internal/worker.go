package internal

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

const (
	PaymentProcessingQueue = "queue:payments"
)

const (
	HealthCheckKeyDefault     = "health-check:default"
	HealthCheckKeyFallback    = "health-check:fallback"
	HealthCheckTicker         = 5 * time.Second
	BackoffTimeEmptyQueue     = 1 * time.Second
	MinAcceptableResponseTime = 200 // in milliseconds
)

type PaymentProcessor struct {
	redis                *redis.Client
	client               *fasthttp.HostClient
	repo                 *PaymentRepository
	healthStatusDefault  atomic.Value
	healthStatusFallback atomic.Value
	workers              int
}

func NewPaymentProcessor(redis *redis.Client, client *fasthttp.HostClient, repo *PaymentRepository, workers int) *PaymentProcessor {
	w := &PaymentProcessor{
		redis:   redis,
		client:  client,
		repo:    repo,
		workers: workers,
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
			"/payments",
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
	path string,
	timeout time.Duration,
	endpoint PaymentEndpoint,
) error {
	// start1 := time.Now()
	payment.UpdateRequestTime()
	raw, err := sonic.ConfigFastest.Marshal(payment)
	if err != nil {
		slog.Error("failed to marshal the payment", "err", err)
		return err
	}

	req := fasthttp.AcquireRequest()
	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(res)

	req.SetRequestURI(path)
	req.SetHost(w.client.Addr)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetBody(raw)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	if err := w.client.DoDeadline(req, res, time.Now().Add(timeout)); err != nil {
		slog.Error("failed to send the request", "err", err)
		return err
	}

	slog.Debug("response from the processor", "res", res, "err", err)

	if res != nil && res.StatusCode() == 422 {
		return nil
	}
	if res != nil && res.StatusCode() == 500 {
		return ErrUnavailableProcessor
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrUnavailableProcessor
	}
	if res != nil && res.StatusCode() != 200 {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}
	if err != nil || res == nil {
		slog.Error("failed to process the request", "err", err, "res", res)
		return ErrUnavailableProcessor
	}

	//start2 := time.Now()
	err = w.repo.Add(PaymentProcessed{
		PaymentRequestProcessor: payment,
		Processed:               endpoint,
	})

	// if time.Since(start1).Milliseconds() > 80 {
	// 	slog.Debug("time of the complete request and db",
	// 		"dbTimeMs", time.Since(start2).Milliseconds(),
	// 		"requestTimeMs", time.Since(start1).Milliseconds(),
	// 		"healthStatusDefault", w.healthStatusDefault.Load().(HealthCheckResponse),
	// 		"healthStatusFallback", w.healthStatusFallback.Load().(HealthCheckResponse),
	// 		"endpoint", endpoint,
	// 		"err", err,
	// 		"requestAt", *payment.RequestedAt,
	// 	)
	// }
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
				if err := w.storeHealthStatus("/payments/service-health", HealthCheckKeyDefault); err != nil {
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
				if err := w.storeHealthStatus("/payments/service-health", HealthCheckKeyFallback); err != nil {
					slog.Debug("failed to update the health check", "err", err)
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

func (w *PaymentProcessor) retrieveHealth(path string) (HealthCheckResponse, error) {
	req := fasthttp.AcquireRequest()
	req.SetRequestURI(path)
	req.Header.SetMethod(fasthttp.MethodGet)

	res := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(res)

	if err := w.client.DoDeadline(req, res, time.Now().Add(time.Second*1)); err != nil {
		slog.Debug("failed to health check", "path", path)
		return HealthCheckResponse{}, err
	}

	var body HealthCheckResponse
	if err := sonic.Unmarshal(res.Body(), &body); err != nil {
		slog.Debug("failed to parse the response", "path", path)
		return HealthCheckResponse{}, err
	}

	return body, nil
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
