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
	return fmt.Sprintf("POINT(%f %f)", l.Lat, l.Lng)
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

	s := strings.TrimSpace(data)

	if !strings.HasPrefix(strings.ToUpper(s), "POINT") {
		return fmt.Errorf("scan error: invalid WKT format, expected POINT(...), got: %q", data)
	}

	s = s[5:]

	s = strings.TrimSpace(s)
	s = strings.Trim(s, "()")
	s = strings.TrimSpace(s)

	_, err := fmt.Sscanf(s, "%f %f", &l.Lng, &l.Lat)
	if err != nil {
		return fmt.Errorf("scan error: parsed string %q from original %q: %w", s, data, err)
	}

	return nil
}

func (l Location) Value() (driver.Value, error) {
	return l.String(), nil
}
