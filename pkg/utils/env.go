package utils

import "os"

func GetEnvOrSetDefault(key string, defaultVal string) string {
	if os.Getenv(key) == "" {
		os.Setenv(key, defaultVal)
		return defaultVal
	}

	return os.Getenv(key)
}
