package auth

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestJWTAuth_GenerateAndValidateToken(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	jwtAuth := NewJWTAuth(
		"test-secret-key",
		1*time.Hour,
		24*time.Hour,
		"test-issuer",
		"test-audience",
		"HS256",
		logger,
	)

	// Test token generation
	token, err := jwtAuth.GenerateToken(
		"user123",
		"testuser",
		"test@example.com",
		[]string{"user", "admin"},
		map[string]string{"department": "engineering"},
	)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Test token validation
	claims, err := jwtAuth.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Verify claims
	if claims.UserID != "user123" {
		t.Errorf("Expected UserID 'user123', got '%s'", claims.UserID)
	}
	if claims.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got '%s'", claims.Username)
	}
	if len(claims.Roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(claims.Roles))
	}
}

func TestJWTAuth_RoleChecking(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	jwtAuth := NewJWTAuth(
		"test-secret-key",
		1*time.Hour,
		24*time.Hour,
		"test-issuer",
		"test-audience",
		"HS256",
		logger,
	)

	claims := &Claims{
		UserID:   "user123",
		Username: "testuser",
		Roles:    []string{"user", "admin"},
	}

	// Test HasRole
	if !jwtAuth.HasRole(claims, "admin") {
		t.Error("Expected user to have admin role")
	}
	if jwtAuth.HasRole(claims, "superuser") {
		t.Error("Expected user to not have superuser role")
	}

	// Test HasAnyRole
	if !jwtAuth.HasAnyRole(claims, []string{"admin", "superuser"}) {
		t.Error("Expected user to have any of the roles")
	}
	if jwtAuth.HasAnyRole(claims, []string{"superuser", "moderator"}) {
		t.Error("Expected user to not have any of the roles")
	}

	// Test HasAllRoles
	if !jwtAuth.HasAllRoles(claims, []string{"user", "admin"}) {
		t.Error("Expected user to have all roles")
	}
	if jwtAuth.HasAllRoles(claims, []string{"user", "admin", "superuser"}) {
		t.Error("Expected user to not have all roles")
	}
}
