package main

import (
	"encoding/json"
	"errors"

	"resty.dev/v3"
)

var (
	ErrRetriesAreOver = errors.New("retries are over")
	ErrInvalidRequest = errors.New("invalid request")
)

type PaymentProcessorAdapter struct {
	clientDefault  *resty.Client
	clientFallback *resty.Client
	defaultUrl     string
	fallbackUrl    string
}

func (a *PaymentProcessorAdapter) Process(p PaymentRequestProcessor) error {
	err := a.process(a.clientDefault, a.defaultUrl+"/payments", p)
	if err == nil {
		return nil
	}

	if err == ErrInvalidRequest {
		return err
	}

	// TODO: Should I retry after being over with both endpoints?
	// Think about it.
	err = a.process(a.clientFallback, a.fallbackUrl+"/payments", p)
	return err
}

func (a *PaymentProcessorAdapter) process(client *resty.Client, url string, p PaymentRequestProcessor) error {
	res, err := client.R().
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetBody(p).
		SetDoNotParseResponse(true).
		Post(url)

	if res.StatusCode() >= 400 && res.StatusCode() < 499 {
		return ErrInvalidRequest
	}

	// after all the retries
	if res.StatusCode() >= 500 {
		return ErrRetriesAreOver
	}

	return err
}

func (a *PaymentProcessorAdapter) Summary(from, to string) (SummaryResponse, error) {
	resDefaultBody, err := a.summary(a.clientDefault, a.defaultUrl+"/admin/payments-summary", from, to)
	if err != nil {
		return SummaryResponse{}, err
	}
	resFallbackBody, err := a.summary(a.clientFallback, a.fallbackUrl+"/admin/payments-summary", from, to)
	if err != nil {
		return SummaryResponse{}, err
	}

	return SummaryResponse{
		DefaultSummary:  resDefaultBody,
		FallbackSummary: resFallbackBody,
	}, nil
}

func (a *PaymentProcessorAdapter) summary(client *resty.Client, url string, from, to string) (SummaryTotalRequestsResponse, error) {
	req := client.R()
	req.SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetHeader("X-Rinha-Token", "123")

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
