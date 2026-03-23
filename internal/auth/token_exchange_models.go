package auth

import "github.com/golang-jwt/jwt/v5"

// MemberData represents the JSON response from the API.
type MemberData struct {
	ID         string `json:"id"`
	Expiration string `json:"expiration"`
}

// UserData represents the JSON structure returned by the API.
type UserData struct {
	ID   string   `json:"id"`
	Role []string `json:"role"`
}

type UserClaims struct {
	UserID string   `json:"userId"`
	Role   []string `json:"role"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	jwt.RegisteredClaims
}
