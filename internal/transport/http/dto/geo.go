package dto

type AddressSuggestion struct {
	Value string  `json:"value"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}

type DaDataResponse struct {
	Suggestions []DaDataSuggestion `json:"suggestions"`
}

type DaDataSuggestion struct {
	Value             string     `json:"value"`
	UnrestrictedValue string     `json:"unrestricted_value"`
	Data              DaDataData `json:"data"`
}

type DaDataData struct {
	GeoLat string `json:"geo_lat"`
	GeoLon string `json:"geo_lon"`
}
