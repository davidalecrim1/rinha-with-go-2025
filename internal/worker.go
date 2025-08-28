package internal

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"net/http"
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
	HealthCheckTicker         = 1 * time.Second
	BackoffTimeEmptyQueue     = 100 * time.Millisecond
	MinAcceptableResponseTime = 200 // in milliseconds
	MinJitterBetweenPayments  = 10  // in milliseconds
	MaxJitterBetweenPayments  = 30  // in milliseconds
)

type PaymentProcessor struct {
	redis                  *redis.Client
	defaultFasthttpClient  *fasthttp.HostClient
	fallbackFasthttpClient *fasthttp.HostClient
	httpClient             *http.Client
	repo                   *PaymentRepository
	healthStatusDefault    atomic.Value
	healthStatusFallback   atomic.Value
	cfg                    *Config
}

func NewPaymentProcessor(redis *redis.Client, defaultFasthttpClient *fasthttp.HostClient, fallbackFasthttpClient *fasthttp.HostClient, httpClient *http.Client, repo *PaymentRepository, cfg *Config) *PaymentProcessor {
	w := &PaymentProcessor{
		redis:                  redis,
		defaultFasthttpClient:  defaultFasthttpClient,
		fallbackFasthttpClient: fallbackFasthttpClient,
		httpClient:             httpClient,
		repo:                   repo,
		cfg:                    cfg,
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
	healthStatusFallback := w.healthStatusFallback.Load().(HealthCheckResponse)

	var err error
	if !healthStatusDefault.Failing && healthStatusDefault.MinResponseTime < MinAcceptableResponseTime {
		err = w.sendPayment(
			payment,
			"/payments",
			time.Second*10,
			PaymentEndpointDefault,
		)
	} else if !healthStatusFallback.Failing && healthStatusFallback.MinResponseTime < MinAcceptableResponseTime {
		err = w.sendPayment(
			payment,
			"/payments",
			time.Second*10,
			PaymentEndpointFallback,
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
	req.Header.SetMethod(fasthttp.MethodPost)
	req.SetBody(raw)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "keep-alive")

	if endpoint == PaymentEndpointDefault {
		req.SetHost(w.defaultFasthttpClient.Addr)
		err = w.defaultFasthttpClient.DoDeadline(req, res, time.Now().Add(timeout))
	} else {
		req.SetHost(w.fallbackFasthttpClient.Addr)
		err = w.fallbackFasthttpClient.DoDeadline(req, res, time.Now().Add(timeout))
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

	err = w.repo.Add(PaymentProcessed{
		PaymentRequestProcessor: payment,
		Processed:               endpoint,
	})

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
				if err := w.storeHealthStatus("http://"+w.cfg.PaymentProcessorDefault+"/payments/service-health", HealthCheckKeyDefault); err != nil {
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
				if err := w.storeHealthStatus("http://"+w.cfg.PaymentProcessorFallback+"/payments/service-health", HealthCheckKeyFallback); err != nil {
					slog.Debug("failed to update the health check", "err", err)
				}
			}
		}
	}()
}

func (w *PaymentProcessor) storeHealthStatus(url string, key string) error {
	res, err := w.retrieveHealth(url)
	if err != nil {
		return err
	}

	body := HealthCheckResponse{
		Failing:         res.Failing,
		MinResponseTime: res.MinResponseTime,
	}

	rawBody, err := sonic.ConfigFastest.Marshal(body)
	if err != nil {
		slog.Debug("failed to encode the json object for redis", "err", err)
		return err
	}

	if err := w.redis.Set(context.Background(), key, rawBody, 0).Err(); err != nil {
		slog.Debug("failed to save health check in redis", "err", err)
		return err
	}

	slog.Debug("updating the health check", "healthCheckStatus", body, "key", key)
	return nil
}

func (w *PaymentProcessor) retrieveHealth(url string) (HealthCheckResponse, error) {
	res, err := w.httpClient.Get(url)
	if err != nil {
		slog.Error("failed to retrieve the health check", "url", url, "err", err)
		return HealthCheckResponse{}, err
	}
	defer res.Body.Close()

	var body HealthCheckResponse
	if err := sonic.ConfigFastest.NewDecoder(res.Body).Decode(&body); err != nil {
		slog.Error("failed to parse the response", "url", url, "err", err)
		return HealthCheckResponse{}, err
	}

	return body, nil
}

func (w *PaymentProcessor) StartWorkers(ctx context.Context) {
	for range w.cfg.NumWorkers {
		go w.run(ctx)
	}

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := w.syncHealthStatus(HealthCheckKeyDefault); err != nil {
				slog.Error("failed update the health check", "err", err)
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(HealthCheckTicker)
		defer ticker.Stop()

		for range ticker.C {
			if err := w.syncHealthStatus(HealthCheckKeyFallback); err != nil {
				slog.Error("failed update the health check", "err", err)
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

			// this is a jitter to avoid overloading the network
			time.Sleep(time.Millisecond * time.Duration(rand.Intn(MaxJitterBetweenPayments-MinJitterBetweenPayments)+MinJitterBetweenPayments))

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

	slog.Info("syncing the health check", "key", key, "healthCheckStatus", healthCheckStatus)

	switch key {
	case HealthCheckKeyDefault:
		w.healthStatusDefault.Store(healthCheckStatus)
	case HealthCheckKeyFallback:
		w.healthStatusFallback.Store(healthCheckStatus)
	}

	return nil
}
