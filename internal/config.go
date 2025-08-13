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

	NumWorkers int

	UnixSocketPath        string
	SummaryUnixSocketPath string

	InitalDatabaseCap int
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

	c.UnixSocketPath = utils.GetEnvOrSetDefault("UNIX_SOCKET", "/var/run/api.sock")
	c.SummaryUnixSocketPath = utils.GetEnvOrSetDefault("SUMMARY_UNIX_SOCKET", "/var/run/summary.sock")

	c.InitalDatabaseCap, _ = strconv.Atoi(utils.GetEnvOrSetDefault("INITAL_DATABASE_CAP", "10000"))
}
