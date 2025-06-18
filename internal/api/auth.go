package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/dreamer-zq/DKNet/internal/config"
)

// AuthContext contains authentication information
type AuthContext struct {
	// Authenticated indicates if the request is authenticated
	Authenticated bool
	// UserID is the authenticated user identifier
	UserID string
	// Roles contains user roles/permissions
	Roles []string
	// Claims contains additional JWT claims
	Claims map[string]interface{}
}

// Authenticator defines the authentication interface
type Authenticator interface {
	// Authenticate validates the JWT token
	Authenticate(ctx context.Context, token string) (*AuthContext, error)

	// Enabled checks if authentication is enabled
	Enabled() bool
}

// authenticator implements the Authenticator interface
type authenticator struct {
	config *config.AuthConfig
	logger *zap.Logger
}

// NewAuthenticator creates a new authenticator
func NewAuthenticator(cfg *config.AuthConfig, logger *zap.Logger) Authenticator {
	return &authenticator{
		config: cfg,
		logger: logger,
	}
}

// Authenticate validates the JWT token
func (a *authenticator) Authenticate(ctx context.Context, token string) (*AuthContext, error) {
	if !a.config.Enabled {
		return &AuthContext{Authenticated: true}, nil
	}

	if token == "" {
		return nil, errors.New("JWT token is required")
	}
	// Remove "Bearer " prefix if present
	tokenString := strings.TrimPrefix(token, "Bearer ")
	jwtToken, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(a.config.JWTSecret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT token: %w", err)
	}

	if !jwtToken.Valid {
		return nil, errors.New("invalid JWT token")
	}

	claims, ok := jwtToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid JWT claims")
	}

	// Validate issuer if configured
	if a.config.JWTIssuer != "" {
		if iss, ok := claims["iss"].(string); !ok || iss != a.config.JWTIssuer {
			return nil, errors.New("invalid JWT issuer")
		}
	}

	// Validate expiration (only if exp claim exists)
	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, errors.New("JWT token has expired")
		}
	}
	// Note: If no "exp" claim exists, the token is considered permanent

	// Extract user information
	userID := ""
	if sub, ok := claims["sub"].(string); ok {
		userID = sub
	}

	roles := []string{}
	if rolesInterface, ok := claims["roles"]; ok {
		if rolesList, ok := rolesInterface.([]any); ok {
			for _, role := range rolesList {
				if roleStr, ok := role.(string); ok {
					roles = append(roles, roleStr)
				}
			}
		}
	}

	return &AuthContext{
		Authenticated: true,
		UserID:        userID,
		Roles:         roles,
		Claims:        claims,
	}, nil
}

// Enabled checks if authentication is enabled
func (a *authenticator) Enabled() bool {
	return a.config.Enabled
}

// AuthContextKey is the key for storing auth context in request context
type AuthContextKey struct{}

// GetAuthContext retrieves the auth context from the request context
func GetAuthContext(ctx context.Context) (*AuthContext, bool) {
	authCtx, ok := ctx.Value(AuthContextKey{}).(*AuthContext)
	return authCtx, ok
}

// SetAuthContext sets the auth context in the request context
func SetAuthContext(ctx context.Context, authCtx *AuthContext) context.Context {
	return context.WithValue(ctx, AuthContextKey{}, authCtx)
}
