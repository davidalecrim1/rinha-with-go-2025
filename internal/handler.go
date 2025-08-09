package internal

import (
	"github.com/bytedance/sonic"
	"github.com/valyala/fasthttp"
)

type PaymentHandler struct {
	adapter *PaymentAdapter
}

func NewPaymentHandler(adapter *PaymentAdapter) *PaymentHandler {
	return &PaymentHandler{
		adapter: adapter,
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
	fromStr := ctx.QueryArgs().Peek("from")
	toStr := ctx.QueryArgs().Peek("to")

	summary, err := h.adapter.Summary(string(fromStr), string(toStr))
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
	ctx.SetContentType("application/json")
	body, err := sonic.Marshal(summary)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetBody(body)
}

func (h *PaymentHandler) Purge(ctx *fasthttp.RequestCtx) {
	tokenStr := string(ctx.Request.Header.Peek("X-Rinha-Token"))

	if tokenStr == "" {
		tokenStr = "123"
	}

	if err := h.adapter.Purge(tokenStr); err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		return
	}

	ctx.SetStatusCode(fasthttp.StatusOK)
}
