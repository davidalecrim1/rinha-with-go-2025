package utils

import "time"

func IsWithInRange(checked, from, to time.Time) bool {
	return (checked.Equal(from) || checked.After(from)) && (checked.Equal(to) || checked.Before(to))
}
