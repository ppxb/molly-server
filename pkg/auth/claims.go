package auth

import (
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID    string `json:"uid"`
	SessionID string `json:"sid"`
	UserName  string `json:"unm"`
	jwt.RegisteredClaims
}
