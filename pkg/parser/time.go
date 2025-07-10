package parser

import (
	"fmt"
	"time"
)

func FormatRFC3339(t time.Time) string {
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02dZ", year, month, day, hour, min, sec)
}
