package domain

import (
	"database/sql/driver"
	"fmt"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkbhex"
)

type Location struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

func (l Location) String() string {
	return fmt.Sprintf("POINT(%f %f)", l.Lng, l.Lat)
}

func (l *Location) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	var hexData string
	switch v := value.(type) {
	case string:
		hexData = v
	case []byte:
		hexData = string(v)
	default:
		return fmt.Errorf("location scan: expected string/[]byte, got %T", value)
	}

	g, err := ewkbhex.Decode(hexData)
	if err != nil {
		return fmt.Errorf("location scan: wkb decode failed: %w", err)
	}

	p, ok := g.(*geom.Point)
	if !ok {
		return fmt.Errorf("location scan: expected Point, got %T", g)
	}

	l.Lng = p.X()
	l.Lat = p.Y()

	return nil
}

func (l Location) Value() (driver.Value, error) {
	return fmt.Sprintf("POINT(%f %f)", l.Lng, l.Lat), nil
}
