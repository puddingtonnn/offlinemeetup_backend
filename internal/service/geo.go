package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/puddingtonnn/offlinemeetup_backend/internal/transport/http/dto"
	"io"
	"net/http"
	"strconv"
)

const (
	daDataSuggestURL   = "https://suggestions.dadata.ru/suggestions/api/4_1/rs/suggest/address"
	daDataGeoLocateURL = "https://suggestions.dadata.ru/suggestions/api/4_1/rs/geolocate/address"
)

type GeoService struct {
	apiKey string
	client *http.Client
}

func NewGeoService(apiKey string) *GeoService {
	return &GeoService{apiKey: apiKey, client: &http.Client{}}
}

func (s *GeoService) SuggestAddress(query string) ([]dto.AddressSuggestion, error) {
	reqBody := map[string]interface{}{
		"query": query,
		"count": 10,
	}

	return s.sendRequest(daDataSuggestURL, reqBody)
}

func (s *GeoService) SuggestByGeo(lat, lon float64) ([]dto.AddressSuggestion, error) {
	reqBody := map[string]interface{}{
		"lat":           lat,
		"lon":           lon,
		"count":         5,
		"radius_meters": 100,
	}
	return s.sendRequest(daDataGeoLocateURL, reqBody)
}

func (s *GeoService) sendRequest(url string, bodyData interface{}) ([]dto.AddressSuggestion, error) {
	jsonBody, _ := json.Marshal(bodyData)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dadata request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	fmt.Println("DEBUG DADATA RAW:", string(bodyBytes))

	resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

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
		lon, _ := strconv.ParseFloat(item.Data.GeoLon, 64)

		result = append(result, dto.AddressSuggestion{
			Value: item.Value,
			Lat:   lat,
			Lon:   lon,
		})
	}
	return result, nil
}
