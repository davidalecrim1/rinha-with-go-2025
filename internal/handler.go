package internal

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
)

type PaymentHandler struct {
	adapter *PaymentAdapter
}

func NewPaymentHandler(adapter *PaymentAdapter) *PaymentHandler {
	return &PaymentHandler{
		adapter: adapter,
	}
}

func (h *PaymentHandler) RegisterRoutes(app *fiber.App) {
	app.Post("/payments", h.Process)
	app.Get("/payments-summary", h.Summary)
	app.Post("/purge-payments", h.Purge)
}

func (h *PaymentHandler) Process(c *fiber.Ctx) error {
	body := c.Body()
	go func() {
		h.adapter.Process(body)
	}()
	return c.SendStatus(http.StatusAccepted)
}

func (h *PaymentHandler) Summary(c *fiber.Ctx) error {
	fromStr := c.Query("from")
	toStr := c.Query("to")

	summary, err := h.adapter.Summary(fromStr, toStr)
	if err != nil {
		return c.SendStatus(http.StatusInternalServerError)
	}

	return c.JSON(summary)
}

func (h *PaymentHandler) Purge(c *fiber.Ctx) error {
	tokenStr := c.Get("X-Rinha-Token")

	if tokenStr == "" {
		tokenStr = "123"
	}

	if err := h.adapter.Purge(tokenStr); err != nil {
		return c.SendStatus(http.StatusInternalServerError)
	}

	return c.SendStatus(http.StatusOK)
}
