package connector

import (
	"errors"
	"strconv"
	"strings"
)

func parsePositiveInt(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		return 0, errors.New("value must be positive")
	}
	return value, nil
}
