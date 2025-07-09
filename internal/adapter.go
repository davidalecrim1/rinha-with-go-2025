package internal

import (
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"resty.dev/v3"
)

var (
	ErrRetriesAreOver = errors.New("retries are over")
	ErrInvalidRequest = errors.New("invalid request")
)

type PoolProcessor struct {
	Queue   chan PaymentRequestProcessor
	Workers int
}

type PaymentProcessorAdapter struct {
	hotPool     PoolProcessor
	coolPool    PoolProcessor
	coldPool    PoolProcessor
	client      *resty.Client
	defaultUrl  string
	fallbackUrl string
}

func NewPaymentProcessorAdapter(
	client *resty.Client,
	defaultUrl string,
	fallbackUrl string,
) *PaymentProcessorAdapter {
	return &PaymentProcessorAdapter{
		hotPool: PoolProcessor{
			Queue:   make(chan PaymentRequestProcessor, 2000),
			Workers: 75,
		},
		coolPool: PoolProcessor{
			Queue:   make(chan PaymentRequestProcessor, 2000),
			Workers: 75,
		},
		coldPool: PoolProcessor{
			Queue:   make(chan PaymentRequestProcessor, 2000),
			Workers: 200,
		},
		client:      client,
		defaultUrl:  defaultUrl,
		fallbackUrl: fallbackUrl,
	}
}

func (a *PaymentProcessorAdapter) StartWorkers() {
	for range a.hotPool.Workers {
		go a.processHotPool()
	}

	for range a.coolPool.Workers {
		go a.processCoolPool()
	}

	for range a.coldPool.Workers {
		go a.processColdPool()
	}

	go func() {
		for {
			time.Sleep(time.Second * 5)
			slog.Info("Status of the worker queue", "hotPoolLength", len(a.hotPool.Queue), "coolPoolLength", len(a.coolPool.Queue), "coldPoolLength", len(a.coldPool.Queue))
		}
	}()
}

func (a *PaymentProcessorAdapter) EnqueuePayment(p PaymentRequestProcessor) {
	a.hotPool.Queue <- p
}

func (a *PaymentProcessorAdapter) processHotPool() {
	for {
		select {
		case payment, ok := <-a.hotPool.Queue:
			if !ok {
				slog.Info("closing the worker given the queue was closed.")
				return
			}

			req := a.client.R().
				SetTimeout(time.Millisecond*100).
				SetRetryCount(2).
				SetRetryWaitTime(50*time.Millisecond).
				SetRetryMaxWaitTime(500*time.Millisecond).
				SetHeader("Connection", "keep-alive").
				SetHeader("Accept-Encoding", "gzip, deflate, br").
				SetBody(payment).
				SetDoNotParseResponse(true)

			err := a.sendPayment(req, a.defaultUrl+"/payments", payment)
			if err != nil {
				slog.Debug("sending to cool pool", "correlationId", payment.CorrelationId)
				a.coolPool.Queue <- payment
			}
		}
	}
}

func (a *PaymentProcessorAdapter) processCoolPool() {
	for {
		select {
		case payment, ok := <-a.coolPool.Queue:
			if !ok {
				slog.Info("closing the worker given the queue was closed.")
				return
			}

			req := a.client.R().
				SetTimeout(time.Millisecond*200).
				SetRetryCount(4).
				SetRetryWaitTime(100*time.Millisecond).
				SetRetryMaxWaitTime(1*time.Second).
				SetHeader("Connection", "keep-alive").
				SetHeader("Accept-Encoding", "gzip, deflate, br").
				SetBody(payment).
				SetDoNotParseResponse(true)

			err := a.sendPayment(req, a.fallbackUrl+"/payments", payment)
			if err == nil {
				continue
			}

			if err != nil {
				slog.Debug("sending to cold pool", "correlationId", payment.CorrelationId)
				a.coldPool.Queue <- payment
			}
		}
	}
}

func (a *PaymentProcessorAdapter) processColdPool() {
	for {
		select {
		case payment, ok := <-a.coldPool.Queue:
			if !ok {
				slog.Info("closing the worker given the queue was closed.")
				return
			}

			maxAttempts := 15
			for attempt := 1; attempt <= maxAttempts; attempt++ {
				req := a.client.R().
					SetTimeout(time.Second*15).
					SetHeader("Connection", "keep-alive").
					SetHeader("Accept-Encoding", "gzip, deflate, br").
					SetBody(payment).
					SetDoNotParseResponse(true)
				err := a.sendPayment(req, a.defaultUrl+"/payments", payment)
				if err == nil {
					slog.Debug("processed as expected", "CorrelationId", payment.CorrelationId)
					break
				}

				req = a.client.R().
					SetTimeout(time.Second*15).
					SetHeader("Connection", "keep-alive").
					SetHeader("Accept-Encoding", "gzip, deflate, br").
					SetBody(payment).
					SetDoNotParseResponse(true)
				err = a.sendPayment(req, a.fallbackUrl+"/payments", payment)
				if err == nil {
					slog.Debug("processed as expected", "CorrelationId", payment.CorrelationId)
					break
				}

				slog.Debug("retrying", "attempt", attempt, "CorrelationId", payment.CorrelationId)
				time.Sleep(time.Second * time.Duration(attempt) * 2)
			}

			slog.Warn("payment wasn't processed in the cold pool. sendint to hot again.", "correlationId", payment.CorrelationId)
			a.hotPool.Queue <- payment
		}
	}
}

func (a *PaymentProcessorAdapter) sendPayment(req *resty.Request, url string, payment PaymentRequestProcessor) error {
	res, err := req.Post(url)
	if res.StatusCode() >= 400 && res.StatusCode() < 499 {
		slog.Debug("The request is invalid", "StatusCode", res.StatusCode(), "correlationId", payment.CorrelationId)
		return nil
	}

	if res.StatusCode() >= 500 {
		return ErrRetriesAreOver
	}

	return err
}

func (a *PaymentProcessorAdapter) Summary(from, to, token string) (SummaryResponse, error) {
	resDefaultBody, err := a.summary(a.defaultUrl+"/admin/payments-summary", from, to, token)
	if err != nil {
		return SummaryResponse{}, err
	}
	resFallbackBody, err := a.summary(a.fallbackUrl+"/admin/payments-summary", from, to, token)
	if err != nil {
		return SummaryResponse{}, err
	}

	return SummaryResponse{
		DefaultSummary:  resDefaultBody,
		FallbackSummary: resFallbackBody,
	}, nil
}

func (a *PaymentProcessorAdapter) summary(url string, from, to, token string) (SummaryTotalRequestsResponse, error) {
	req := a.client.R()
	req.SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetHeader("X-Rinha-Token", token)

	if from != "" && to != "" {
		req.SetQueryParam("from", from)
		req.SetQueryParam("to", to)
	}

	res, err := req.Get(url)
	if err != nil {
		return SummaryTotalRequestsResponse{}, err
	}
	defer res.Body.Close()
	var resBody SummaryTotalRequestsResponse
	if err := json.NewDecoder(res.Body).Decode(&resBody); err != nil {
		return SummaryTotalRequestsResponse{}, err
	}

	return resBody, nil
}

func (a *PaymentProcessorAdapter) Purge(token string) error {
	if err := a.purge(a.defaultUrl+"/admin/purge-payments", token); err != nil {
		return err
	}
	if err := a.purge(a.fallbackUrl+"/admin/purge-payments", token); err != nil {
		return err
	}

	return nil
}

func (a *PaymentProcessorAdapter) purge(url string, token string) error {
	res, err := a.client.R().
		SetHeader("X-Rinha-Token", token).
		Post(url)
	if err != nil {
		slog.Error("failed to purge the api", "error", err, "url", url)
		return err
	}

	if res.StatusCode() != 200 {
		return ErrInvalidRequest
	}

	return nil
}
