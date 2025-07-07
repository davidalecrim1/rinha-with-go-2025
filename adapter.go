package main

import (
	"encoding/json"
	"errors"

	"resty.dev/v3"
)

var ErrRetriesAreOver = errors.New("retries are over")

type PaymentProcessorAdapter struct {
	client      *resty.Client
	defaultUrl  string
	fallbackUrl string
}

func (a *PaymentProcessorAdapter) Process(p PaymentRequestProcessor) error {
	res, err := a.client.R().
		SetHeader("Connection", "keep-alive").
		SetHeader("Accept-Encoding", "gzip, deflate, br").
		SetBody(p).
		SetDoNotParseResponse(true).
		Post(a.defaultUrl + "/payments")
	if err != nil {
		return err
	}
	// after all the retries
	if res.StatusCode() != 500 {
		return ErrRetriesAreOver
	}

	// I won't care about the response body if there is not an error
	return nil
}

func (a *PaymentProcessorAdapter) Summary(from, to string) (SummaryResponse, error) {
	resDefaultBody, err := a.summary(a.defaultUrl+"/admin/payments-summary", from, to)
	if err != nil {
		return SummaryResponse{}, err
	}
	resFallbackBody, err := a.summary(a.fallbackUrl+"/admin/payments-summary", from, to)
	if err != nil {
		return SummaryResponse{}, err
	}

	return SummaryResponse{
		DefaultSummary:  resDefaultBody,
		FallbackSummary: resFallbackBody,
	}, nil
}

func (a *PaymentProcessorAdapter) summary(url string, from, to string) (SummaryTotalRequestsResponse, error) {
	req := a.client.R()
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
