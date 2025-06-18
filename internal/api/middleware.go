package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// HTTPAuthMiddleware creates a Gin middleware for HTTP authentication
func HTTPAuthMiddleware(auth Authenticator, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !auth.Enabled() {
			c.Next()
			return
		}

		// Extract token from request (may be empty)
		token := extractTokenFromHTTPRequest(c.Request)

		// Authenticate - the authenticator will handle whether auth is enabled
		authCtx, err := auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			logger.Warn("HTTP authentication failed",
				zap.String("path", c.Request.URL.Path),
				zap.String("method", c.Request.Method),
				zap.Error(err))

			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication failed",
				"code":  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		// Set auth context in request context
		ctx := SetAuthContext(c.Request.Context(), authCtx)
		c.Request = c.Request.WithContext(ctx)

		logger.Debug("HTTP authentication successful",
			zap.String("path", c.Request.URL.Path),
			zap.String("user_id", authCtx.UserID),
			zap.Strings("roles", authCtx.Roles))

		c.Next()
	}
}

// GRPCAuthInterceptor creates a gRPC unary interceptor for authentication
func GRPCAuthInterceptor(auth Authenticator, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if !auth.Enabled() {
			return handler(ctx, req)
		}

		// Extract token from gRPC metadata (if available)
		token, err := extractTokenFromGRPCContext(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "Authentication token required")
		}

		// Authenticate - this will check if auth is enabled
		authCtx, err := auth.Authenticate(ctx, token)
		if err != nil {
			logger.Warn("gRPC authentication failed",
				zap.String("method", info.FullMethod),
				zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, "Authentication failed: %v", err)
		}

		// Set auth context in request context
		ctx = SetAuthContext(ctx, authCtx)

		logger.Debug("gRPC authentication successful",
			zap.String("method", info.FullMethod),
			zap.String("user_id", authCtx.UserID),
			zap.Strings("roles", authCtx.Roles))

		return handler(ctx, req)
	}
}

// GRPCAuthStreamInterceptor creates a gRPC stream interceptor for authentication
func GRPCAuthStreamInterceptor(auth Authenticator, logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Try to authenticate first - the authenticator will handle whether auth is enabled
		var token string
		var err error

		// Extract token from gRPC metadata (if available)
		token, err = extractTokenFromGRPCContext(ss.Context())
		if err != nil {
			// If token extraction fails, pass empty token to authenticator
			// The authenticator will decide whether this is acceptable based on config
			token = ""
		}

		// Authenticate - this will check if auth is enabled
		authCtx, err := auth.Authenticate(ss.Context(), token)
		if err != nil {
			logger.Warn("gRPC stream authentication failed",
				zap.String("method", info.FullMethod),
				zap.Error(err))
			return status.Errorf(codes.Unauthenticated, "Authentication failed: %v", err)
		}

		// Create a new context with auth information
		ctx := SetAuthContext(ss.Context(), authCtx)

		logger.Debug("gRPC stream authentication successful",
			zap.String("method", info.FullMethod),
			zap.String("user_id", authCtx.UserID),
			zap.Strings("roles", authCtx.Roles))

		// Wrap the server stream with the new context
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		return handler(srv, wrappedStream)
	}
}

// wrappedServerStream wraps grpc.ServerStream to override context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context returns the wrapped context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// extractTokenFromHTTPRequest extracts JWT token from HTTP request
func extractTokenFromHTTPRequest(req *http.Request) string {
	// Try Authorization header
	if auth := req.Header.Get("Authorization"); auth != "" {
		return auth
	}

	return ""
}

// extractTokenFromGRPCContext extracts JWT token from gRPC context metadata
func extractTokenFromGRPCContext(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Errorf(codes.Unauthenticated, "metadata not found")
	}

	// Try authorization header
	if values := md.Get("authorization"); len(values) > 0 {
		return values[0], nil
	}

	return "", status.Errorf(codes.Unauthenticated, "JWT token not found")
}

// RequireRole creates a middleware that requires specific roles
func RequireRole(requiredRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authCtx, ok := GetAuthContext(c.Request.Context())
		if !ok || !authCtx.Authenticated {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
				"code":  "UNAUTHORIZED",
			})
			c.Abort()
			return
		}

		// Check if user has any of the required roles
		hasRole := false
		for _, userRole := range authCtx.Roles {
			for _, requiredRole := range requiredRoles {
				if userRole == requiredRole {
					hasRole = true
					break
				}
			}
			if hasRole {
				break
			}
		}

		if !hasRole {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient permissions",
				"code":  "FORBIDDEN",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
