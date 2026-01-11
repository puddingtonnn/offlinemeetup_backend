package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"net/http"
	"strconv"
)

const daDataURL = "https://suggestions.dadata.ru/suggestions/api/4_1/rs/suggest/address"

type GeoService struct {
	apiKey string
}

func NewGeoService(apiKey string) *GeoService {
	return &GeoService{apiKey: apiKey}
}

func (s *GeoService) SuggestAddress(query string) ([]dto.AddressSuggestion, error) {
	if query == "" {
		return nil, nil
	}

	reqBody := map[string]interface{}{
		"query": query,
		"count": 10,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", daDataURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+s.apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dadata request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dadata returned status: %s", resp.StatusCode)
	}

	var daDataResp dto.DaDataResponse
	if err := json.NewDecoder(resp.Body).Decode(&daDataResp); err != nil {
		return nil, fmt.Errorf("failed to decode dadata response: %w", err)
	}

	result := make([]dto.AddressSuggestion, 0, len(daDataResp.Suggestions))

	for _, item := range daDataResp.Suggestions {
		lat, _ := strconv.ParseFloat(item.Data.GeoLat, 64)
		lng, _ := strconv.ParseFloat(item.Data.GeoLon, 64)

		result = append(result, dto.AddressSuggestion{
			Value: item.Value, //
			Lat:   lat,
			Lng:   lng,
		})
	}

	return result, nil
}
