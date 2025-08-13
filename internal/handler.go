package internal

import (
	"log/slog"

	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

type PaymentHandler struct {
	adapter          *PaymentAdapter
	unixSocketClient *fasthttp.HostClient
}

func NewPaymentHandler(adapter *PaymentAdapter, unixSocketClient *fasthttp.HostClient) *PaymentHandler {
	return &PaymentHandler{
		adapter:          adapter,
		unixSocketClient: unixSocketClient,
	}
}

func (h *PaymentHandler) Router(ctx *fasthttp.RequestCtx) {
	switch string(ctx.Path()) {
	case "/payments":
		if ctx.IsPost() {
			h.Process(ctx)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}
	case "/payments-summary":
		if ctx.IsGet() {
			h.Summary(ctx)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}
	case "/purge-payments":
		if ctx.IsPost() {
			h.Purge(ctx)
		} else {
			ctx.SetStatusCode(fasthttp.StatusMethodNotAllowed)
		}

	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}
}

func (h *PaymentHandler) Process(ctx *fasthttp.RequestCtx) {
	body := ctx.PostBody()
	go func() {
		h.adapter.Process(body)
	}()
	ctx.SetStatusCode(fasthttp.StatusAccepted)
}

func (h *PaymentHandler) Summary(ctx *fasthttp.RequestCtx) {
	req := &ctx.Request
	resp := &ctx.Response

	if err := h.unixSocketClient.Do(req, resp); err != nil {
		slog.Error("error forwarding request to summary repository", "error", err)
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}
}

func (h *PaymentHandler) Purge(ctx *fasthttp.RequestCtx) {
	tokenStr := string(ctx.Request.Header.Peek("X-Rinha-Token"))

	go func() {
		if tokenStr == "" {
			tokenStr = "123"
		}

		if err := h.adapter.Purge(tokenStr); err != nil {
			slog.Error("failed to purge payments", "error", err)
		}
	}()

	ctx.SetStatusCode(fasthttp.StatusOK)
}

type SummaryHandler struct {
	repo *PaymentRepository
}

func NewSummaryHandler(repo *PaymentRepository) *SummaryHandler {
	return &SummaryHandler{repo: repo}
}

func (h *SummaryHandler) GetSummary(ctx *fasthttp.RequestCtx) {
	from := ctx.QueryArgs().Peek("from")
	to := ctx.QueryArgs().Peek("to")

	summary, err := h.repo.Summary(string(from), string(to))
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		slog.Error("failed to get summary", "error", err)
		return
	}

	body, err := sonic.ConfigFastest.Marshal(summary)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		slog.Error("failed to marshal summary", "error", err)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/json")
	ctx.SetBody(body)
}
