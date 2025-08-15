package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Claims represents JWT claims
type Claims struct {
	UserID   string            `json:"user_id"`
	Username string            `json:"username"`
	Email    string            `json:"email"`
	Roles    []string          `json:"roles"`
	Metadata map[string]string `json:"metadata,omitempty"`
	jwt.RegisteredClaims
}

// JWTAuth handles JWT authentication
type JWTAuth struct {
	secret         []byte
	expirationTime time.Duration
	refreshTime    time.Duration
	issuer         string
	audience       string
	algorithm      string
	logger         *zap.Logger
}

// NewJWTAuth creates a new JWT authenticator
func NewJWTAuth(secret string, expirationTime, refreshTime time.Duration, issuer, audience, algorithm string, logger *zap.Logger) *JWTAuth {
	return &JWTAuth{
		secret:         []byte(secret),
		expirationTime: expirationTime,
		refreshTime:    refreshTime,
		issuer:         issuer,
		audience:       audience,
		algorithm:      algorithm,
		logger:         logger,
	}
}

// GenerateToken generates a new JWT token
func (j *JWTAuth) GenerateToken(userID, username, email string, roles []string, metadata map[string]string) (string, error) {
	now := time.Now()
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Email:    email,
		Roles:    roles,
		Metadata: metadata,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(j.expirationTime)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    j.issuer,
			Audience:  []string{j.audience},
			Subject:   userID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(j.secret)
	if err != nil {
		j.logger.Error("Failed to sign JWT token", zap.Error(err))
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	j.logger.Debug("Generated JWT token", zap.String("user_id", userID), zap.String("username", username))
	return tokenString, nil
}

// ValidateToken validates a JWT token and returns claims
func (j *JWTAuth) ValidateToken(tokenString string) (*Claims, error) {
	// Remove "Bearer " prefix if present
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secret, nil
	})

	if err != nil {
		j.logger.Debug("Token validation failed", zap.Error(err))
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		j.logger.Debug("Token validated successfully", zap.String("user_id", claims.UserID))
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// RefreshToken generates a new token if the current one is close to expiration
func (j *JWTAuth) RefreshToken(tokenString string) (string, error) {
	claims, err := j.ValidateToken(tokenString)
	if err != nil {
		return "", fmt.Errorf("invalid token for refresh: %w", err)
	}

	// Check if token is close to expiration (within refresh window)
	now := time.Now()
	expirationTime := claims.ExpiresAt.Time
	refreshThreshold := expirationTime.Add(-j.refreshTime)

	if now.Before(refreshThreshold) {
		return "", fmt.Errorf("token is not close to expiration")
	}

	// Generate new token with same claims but new expiration
	return j.GenerateToken(claims.UserID, claims.Username, claims.Email, claims.Roles, claims.Metadata)
}

// ExtractTokenFromHeader extracts token from Authorization header
func (j *JWTAuth) ExtractTokenFromHeader(authHeader string) (string, error) {
	if authHeader == "" {
		return "", fmt.Errorf("authorization header is empty")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fmt.Errorf("invalid authorization header format")
	}

	return parts[1], nil
}

// HasRole checks if the user has a specific role
func (j *JWTAuth) HasRole(claims *Claims, role string) bool {
	for _, userRole := range claims.Roles {
		if userRole == role {
			return true
		}
	}
	return false
}

// HasAnyRole checks if the user has any of the specified roles
func (j *JWTAuth) HasAnyRole(claims *Claims, roles []string) bool {
	for _, requiredRole := range roles {
		if j.HasRole(claims, requiredRole) {
			return true
		}
	}
	return false
}

// HasAllRoles checks if the user has all of the specified roles
func (j *JWTAuth) HasAllRoles(claims *Claims, roles []string) bool {
	for _, requiredRole := range roles {
		if !j.HasRole(claims, requiredRole) {
			return false
		}
	}
	return true
}

// GetTokenInfo returns token information without validation
func (j *JWTAuth) GetTokenInfo(tokenString string) (*Claims, error) {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	// Parse token without validation
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}
