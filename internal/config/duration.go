package config

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

var durationRegex = regexp.MustCompile(`^(\d+)(d|h|m|s)$`)

// ParseDuration parses a duration string like "3d", "7d", "14d", "24h".
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}

	matches := durationRegex.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid duration format: %q (expected format like '3d', '7d', '24h')", s)
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid duration value: %q", s)
	}

	unit := matches[2]
	switch unit {
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "h":
		return time.Duration(value) * time.Hour, nil
	case "m":
		return time.Duration(value) * time.Minute, nil
	case "s":
		return time.Duration(value) * time.Second, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %q", unit)
	}
}
