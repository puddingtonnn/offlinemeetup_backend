package middleware

import (
	"context"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"os"
	"strings"
)

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization header is missing", http.StatusUnauthorized)
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == "" {
			http.Error(w, "invalid token format", http.StatusUnauthorized)
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			secret := os.Getenv("JWT_SECRET_KEY")
			if secret == "" {
				return nil, fmt.Errorf("missing JWT secret key")
			}

			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, "invalid token claims", http.StatusUnauthorized)
			return
		}

		userIDFloat, ok := claims["userID"].(float64)
		if !ok {
			http.Error(w, "invalid user id in token", http.StatusUnauthorized)
			return
		}
		userID := int64(userIDFloat)

		ctx := context.WithValue(r.Context(), UserIDKey, userID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
