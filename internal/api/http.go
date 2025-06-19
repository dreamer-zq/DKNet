package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	healthv1 "github.com/dreamer-zq/DKNet/proto/health/v1"
	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// startHTTPServer starts the HTTP server
func (s *Server) startHTTPServer() error {
	// Set Gin mode
	gin.SetMode(gin.ReleaseMode)

	// Create Gin router
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// Setup routes
	s.setupHTTPRoutes(router)

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", s.config.Server.HTTP.Host, s.config.Server.HTTP.Port)
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		var err error
		if s.config.Security.TLSEnabled {
			err = s.httpServer.ListenAndServeTLS(s.config.Security.CertFile, s.config.Security.KeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	return nil
}

// stopHTTPServer stops the HTTP server
func (s *Server) stopHTTPServer() error {
	if s.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.httpServer.Shutdown(ctx)
}

// setupHTTPRoutes sets up HTTP routes
func (s *Server) setupHTTPRoutes(router *gin.Engine) {
	// Health check (excluded from auth)
	router.GET(HealthPath, s.healthHandler)

	// TSS operations with authentication
	api := router.Group(APIVersionPrefix)
	api.Use(HTTPAuthMiddleware(s.authenticator, s.logger))
	api.POST(KeygenPath, s.keygenHandler)
	api.POST(SignPath, s.signHandler)
	api.POST(ResharePath, s.reshareHandler)

	// Operations
	api.GET(OperationPathPattern, s.getOperationHandler)
}

// healthHandler handles health check requests
func (s *Server) healthHandler(c *gin.Context) {
	resp := &healthv1.CheckResponse{
		Status:    healthv1.HealthStatus_HEALTH_STATUS_SERVING,
		Timestamp: timestamppb.Now(),
		Details:   "DKNet is healthy",
		Metadata: map[string]string{
			"service": "dknet",
			"version": "1.0.0",
		},
	}

	c.JSON(http.StatusOK, resp)
}

// keygenHandler handles keygen requests
func (s *Server) keygenHandler(c *gin.Context) {
	var req tssv1.StartKeygenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use background context for async TSS operations to avoid HTTP timeout cancellation
	operation, err := s.tssService.StartKeygen(
		context.Background(),
		req.OperationId,
		int(req.Threshold),
		req.Participants,
	)
	if err != nil {
		s.logger.Error("Failed to start keygen", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &tssv1.StartKeygenResponse{
		OperationId: operation.ID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}

	c.JSON(http.StatusAccepted, resp)
}

// signHandler handles signing requests
func (s *Server) signHandler(c *gin.Context) {
	var req tssv1.StartSigningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use background context for async TSS operations to avoid HTTP timeout cancellation
	operation, err := s.tssService.StartSigning(
		context.Background(),
		req.OperationId,
		req.Message,
		req.KeyId,
		req.Participants,
	)
	if err != nil {
		s.logger.Error("Failed to start signing", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &tssv1.StartSigningResponse{
		OperationId: operation.ID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}

	c.JSON(http.StatusAccepted, resp)
}

// reshareHandler handles resharing requests
func (s *Server) reshareHandler(c *gin.Context) {
	var req tssv1.StartResharingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use background context for async TSS operations to avoid HTTP timeout cancellation
	operation, err := s.tssService.StartResharing(
		context.Background(),
		req.OperationId,
		req.KeyId,
		int(req.NewThreshold),
		req.OldParticipants,
		req.NewParticipants,
	)
	if err != nil {
		s.logger.Error("Failed to start resharing", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &tssv1.StartResharingResponse{
		OperationId: operation.ID,
		Status:      convertOperationStatus(operation.Status),
		CreatedAt:   timestamppb.New(operation.CreatedAt),
	}

	c.JSON(http.StatusAccepted, resp)
}

// getOperationHandler handles get operation requests
func (s *Server) getOperationHandler(c *gin.Context) {
	operationID := c.Param("operation_id")

	// First try to get from active operations in memory
	operation, exists := s.tssService.GetOperation(operationID)
	if exists {
		operation.RLock()
		defer operation.RUnlock()

		c.JSON(http.StatusOK, buildOperationResponse(operation))
		return
	}

	// If not found in memory, try persistent storage
	ctx := context.Background()
	operationData, err := s.tssService.GetOperationData(ctx, operationID)
	if err != nil {
		s.logger.Warn("Operation not found", zap.String("operation_id", operationID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}

	resp := buildOperationResponseFromStorage(operationData)

	c.JSON(http.StatusOK, resp)
}
