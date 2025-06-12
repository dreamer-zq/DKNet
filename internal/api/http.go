package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dreamer-zq/DKNet/internal/tss"
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
	addr := fmt.Sprintf("%s:%d", s.config.HTTP.Host, s.config.HTTP.Port)
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
	// Health check
	router.GET("/health", s.healthHandler)

	// TSS operations
	api := router.Group("/api/v1")
	{
		api.POST("/keygen", s.keygenHandler)
		api.POST("/sign", s.signHandler)
		api.POST("/reshare", s.reshareHandler)

		// Operations
		api.GET("/operations/:operation_id", s.getOperationHandler)
		api.DELETE("/operations/:operation_id", s.cancelOperationHandler)
	}
}

// healthHandler handles health check requests
func (s *Server) healthHandler(c *gin.Context) {
	resp := &healthv1.CheckResponse{
		Status:    healthv1.HealthStatus_HEALTH_STATUS_SERVING,
		Timestamp: timestamppb.Now(),
		Details:   "DKNet is healthy",
		Metadata: map[string]string{
			"service": "tss-server",
			"version": "1.0.0",
		},
	}

	c.JSON(http.StatusOK, resp)
}

// keygenHandler handles keygen requests
func (s *Server) keygenHandler(c *gin.Context) {
	var req tss.KeygenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use background context for async TSS operations to avoid HTTP timeout cancellation
	operation, err := s.tssService.StartKeygen(context.Background(), &req)
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
	var req tss.SigningRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use background context for async TSS operations to avoid HTTP timeout cancellation
	operation, err := s.tssService.StartSigning(context.Background(), &req)
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
	var req tss.ResharingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Use background context for async TSS operations to avoid HTTP timeout cancellation
	operation, err := s.tssService.StartResharing(context.Background(), &req)
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

		resp := &tssv1.GetOperationResponse{
			OperationId: operation.ID,
			Type:        convertOperationType(operation.Type),
			SessionId:   operation.SessionID,
			Status:      convertOperationStatus(operation.Status),
			CreatedAt:   timestamppb.New(operation.CreatedAt),
		}

		// Add participants
		for _, p := range operation.Participants {
			resp.Participants = append(resp.Participants, p.Id)
		}

		// Add completion time if available
		if operation.CompletedAt != nil {
			resp.CompletedAt = timestamppb.New(*operation.CompletedAt)
		}

		// Add error if available
		if operation.Error != nil {
			errMsg := operation.Error.Error()
			resp.Error = &errMsg
		}

		// Add result based on operation type
		if operation.Result != nil {
			switch operation.Type {
			case tss.OperationKeygen:
				if result, ok := operation.Result.(*tss.KeygenResult); ok {
					resp.Result = &tssv1.GetOperationResponse_KeygenResult{
						KeygenResult: &tssv1.KeygenResult{
							PublicKey: result.PublicKey,
							KeyId:     result.KeyID,
						},
					}
				}
			case tss.OperationSigning:
				if result, ok := operation.Result.(*tss.SigningResult); ok {
					resp.Result = &tssv1.GetOperationResponse_SigningResult{
						SigningResult: &tssv1.SigningResult{
							Signature: result.Signature,
							R:         result.R,
							S:         result.S,
						},
					}
				}
			case tss.OperationResharing:
				if result, ok := operation.Result.(*tss.KeygenResult); ok {
					resp.Result = &tssv1.GetOperationResponse_ResharingResult{
						ResharingResult: &tssv1.KeygenResult{
							PublicKey: result.PublicKey,
							KeyId:     result.KeyID,
						},
					}
				}
			}
		}

		// Add original request
		if operation.Request != nil {
			switch req := operation.Request.(type) {
			case *tss.KeygenRequest:
				resp.Request = &tssv1.GetOperationResponse_KeygenRequest{
					KeygenRequest: &tssv1.StartKeygenRequest{
						Threshold:    int32(req.Threshold),
						Parties:      int32(req.Parties),
						Participants: req.Participants,
					},
				}
			case *tss.SigningRequest:
				resp.Request = &tssv1.GetOperationResponse_SigningRequest{
					SigningRequest: &tssv1.StartSigningRequest{
						Message:      req.Message,
						KeyId:        req.KeyID,
						Participants: req.Participants,
					},
				}
			case *tss.ResharingRequest:
				resp.Request = &tssv1.GetOperationResponse_ResharingRequest{
					ResharingRequest: &tssv1.StartResharingRequest{
						KeyId:           req.KeyID,
						NewThreshold:    int32(req.NewThreshold),
						NewParties:      int32(req.NewParties),
						OldParticipants: req.OldParticipants,
						NewParticipants: req.NewParticipants,
					},
				}
			}
		}

		c.JSON(http.StatusOK, resp)
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

	resp := &tssv1.GetOperationResponse{
		OperationId:  operationData.ID,
		Type:         convertOperationType(operationData.Type),
		SessionId:    operationData.SessionID,
		Status:       convertOperationStatus(operationData.Status),
		Participants: operationData.Participants,
		CreatedAt:    timestamppb.New(operationData.CreatedAt),
	}

	// Add completion time if available
	if operationData.CompletedAt != nil {
		resp.CompletedAt = timestamppb.New(*operationData.CompletedAt)
	}

	// Add error if available
	if operationData.Error != "" {
		resp.Error = &operationData.Error
	}

	// Add result based on operation type if available
	if operationData.Result != nil {
		switch operationData.Type {
		case tss.OperationKeygen:
			if result, ok := operationData.Result.(*tss.KeygenResult); ok {
				resp.Result = &tssv1.GetOperationResponse_KeygenResult{
					KeygenResult: &tssv1.KeygenResult{
						PublicKey: result.PublicKey,
						KeyId:     result.KeyID,
					},
				}
			}
		case tss.OperationSigning:
			if result, ok := operationData.Result.(*tss.SigningResult); ok {
				resp.Result = &tssv1.GetOperationResponse_SigningResult{
					SigningResult: &tssv1.SigningResult{
						Signature: result.Signature,
						R:         result.R,
						S:         result.S,
					},
				}
			}
		case tss.OperationResharing:
			if result, ok := operationData.Result.(*tss.KeygenResult); ok {
				resp.Result = &tssv1.GetOperationResponse_ResharingResult{
					ResharingResult: &tssv1.KeygenResult{
						PublicKey: result.PublicKey,
						KeyId:     result.KeyID,
					},
				}
			}
		}
	}

	// Add original request if available
	if operationData.Request != nil {
		switch req := operationData.Request.(type) {
		case *tss.KeygenRequest:
			resp.Request = &tssv1.GetOperationResponse_KeygenRequest{
				KeygenRequest: &tssv1.StartKeygenRequest{
					Threshold:    int32(req.Threshold),
					Parties:      int32(req.Parties),
					Participants: req.Participants,
				},
			}
		case *tss.SigningRequest:
			resp.Request = &tssv1.GetOperationResponse_SigningRequest{
				SigningRequest: &tssv1.StartSigningRequest{
					Message:      req.Message,
					KeyId:        req.KeyID,
					Participants: req.Participants,
				},
			}
		case *tss.ResharingRequest:
			resp.Request = &tssv1.GetOperationResponse_ResharingRequest{
				ResharingRequest: &tssv1.StartResharingRequest{
					KeyId:           req.KeyID,
					NewThreshold:    int32(req.NewThreshold),
					NewParties:      int32(req.NewParties),
					OldParticipants: req.OldParticipants,
					NewParticipants: req.NewParticipants,
				},
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// cancelOperationHandler handles cancel operation requests
func (s *Server) cancelOperationHandler(c *gin.Context) {
	operationID := c.Param("operation_id")

	if err := s.tssService.CancelOperation(operationID); err != nil {
		s.logger.Error("Failed to cancel operation", zap.String("operation_id", operationID), zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp := &tssv1.CancelOperationResponse{
		Message: "operation cancelled",
	}

	c.JSON(http.StatusOK, resp)
}
