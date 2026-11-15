package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Incept10n/coffeely/backend/internal/config"
)

const jwtIssuer = "coffeely"

// Claims — полезная нагрузка access-токена.
type Claims struct {
	Role  string `json:"role"`
	Email string `json:"email,omitempty"`
	jwt.RegisteredClaims
}

// IssueAccessToken подписывает HS256 access-токен (TTL из конфига, 15 минут).
func IssueAccessToken(cfg config.Config, userID, role, email string) (string, error) {
	now := time.Now()
	jti, err := randState()
	if err != nil {
		return "", err
	}
	claims := Claims{
		Role:  role,
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			Issuer:    jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(cfg.AccessTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecret))
}

// ParseAccessToken проверяет подпись, метод, издателя и срок; возвращает claims.
func ParseAccessToken(cfg config.Config, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return []byte(cfg.JWTSecret), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(jwtIssuer))
	if err != nil {
		return nil, fmt.Errorf("auth: access-токен: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("auth: access-токен недействителен")
	}
	return claims, nil
}

// NewRefreshToken создаёт одноразовый refresh-токен: клиенту — открытое значение,
// в БД — только SHA-256 хеш.
func NewRefreshToken() (token, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("auth: refresh token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	hash = hex.EncodeToString(sum[:])
	return token, hash, nil
}

// HashRefreshToken — хеш открытого refresh-токена для поиска в БД.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
