package internal

import (
	"log/slog"
	"strconv"

	"rinha-with-go-2025/pkg/utils"
)

type Config struct {
	PaymentProcessorDefault        string
	PaymentProcessorFallback       string
	MonitorHealthPaymentProcessors bool

	EnableProfiling bool
	LogLevel        slog.Level
	RedisAddr       string

	NumWorkers            int
	LengthProcessorBuffer int

	UnixSocketPath string
}

func NewConfig() *Config {
	return &Config{}
}

func (c *Config) Load() {
	c.PaymentProcessorDefault = utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_DEFAULT", "localhost:8001")
	c.PaymentProcessorFallback = utils.GetEnvOrSetDefault("PAYMENT_PROCESSOR_URL_FALLBACK", "localhost:8002")
	c.MonitorHealthPaymentProcessors = utils.GetEnvOrSetDefault("MONITOR_HEALTH", "false") == "true"

	c.EnableProfiling = utils.GetEnvOrSetDefault("ENABLE_PROFILING", "false") == "true"
	c.LogLevel = slog.LevelInfo
	c.RedisAddr = utils.GetEnvOrSetDefault("REDIS_ADDR", "localhost:6379")

	c.NumWorkers, _ = strconv.Atoi(utils.GetEnvOrSetDefault("WORKERS", "15"))
	c.LengthProcessorBuffer, _ = strconv.Atoi(utils.GetEnvOrSetDefault("LENGTH_PROCESSOR_BUFFER", "6000"))

	c.UnixSocketPath = utils.GetEnvOrSetDefault("UNIX_SOCKET", "/var/run/api.sock")
}
