package dto

import "time"

type UserResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// AuthTokensResponse is the login/refresh payload: a short-lived access token
// plus a long-lived refresh token. ExpiresIn is the access token's lifetime in
// seconds so the client knows when to refresh.
type AuthTokensResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}
