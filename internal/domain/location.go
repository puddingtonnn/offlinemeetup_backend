package domain

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func (l Location) String() string {
	return fmt.Sprintf("POINT(%f, %f)", l.Lat, l.Lng)
}

func (l Location) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	var data string
	switch v := value.(type) {
	case []byte:
		data = string(v)
	case string:
		data = v
	default:
		return fmt.Errorf("failed to scan Location: expected string or []byte, got %T", value)
	}

	trimmed := strings.TrimSpace(data)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "POINT") {
		return fmt.Errorf("invalid location format: %s", data)
	}

	trimmed = strings.TrimLeft(trimmed, "POINTpoint")
	trimmed = strings.Trim(trimmed, " ()")

	_, err := fmt.Sscanf(trimmed, "%f,%f", &l.Lat, &l.Lng)
	return err
}

func (l Location) Value() (driver.Value, error) {
	return l.String(), nil
}
