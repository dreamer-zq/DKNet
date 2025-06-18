package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

var (
	nodeAddr string
	nodeID   string
	jwtToken string
	logger   *zap.Logger
)

func main() {
	// Simple logger setup
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	var err error
	logger, err = config.Build()
	if err != nil {
		log.Fatal("Failed to setup logger:", err)
	}
	defer logger.Sync()

	rootCmd := &cobra.Command{
		Use:   "dknet-mcp",
		Short: "DKNet MCP Server - Bridge between LLM and existing DKNet cluster",
		Long: `DKNet MCP Server connects to an existing DKNet cluster and exposes
TSS operations as MCP tools for LLM clients.

Example:
  dknet-mcp --node-addr localhost:9095 --node-id 12D3KooWExample...`,
		RunE: runMCPServer,
	}

	rootCmd.PersistentFlags().StringVar(&nodeAddr, "node-addr", "localhost:9095", "DKNet node gRPC address")
	rootCmd.PersistentFlags().StringVar(&nodeID, "node-id", "", "Node ID for X-Node-ID header (required)")
	rootCmd.PersistentFlags().StringVar(&jwtToken, "jwt-token", "", "JWT token for authentication (if required)")
	rootCmd.MarkPersistentFlagRequired("node-id")

	if err := rootCmd.Execute(); err != nil {
		logger.Fatal("Command execution failed", zap.Error(err))
	}
}

func runMCPServer(cmd *cobra.Command, args []string) error {
	logger.Info("Starting DKNet MCP Server",
		zap.String("node_address", nodeAddr),
		zap.String("node_id", nodeID),
		zap.Bool("jwt_enabled", jwtToken != ""))

	// Create gRPC connection to DKNet node
	conn, err := grpc.NewClient(nodeAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to DKNet node: %w", err)
	}
	defer conn.Close()

	// Test connection
	tssClient := tssv1.NewTSSServiceClient(conn)
	ctx := contextWithAuth(context.Background())
	_, err = tssClient.GetOperation(ctx, &tssv1.GetOperationRequest{
		OperationId: "test-connection",
	})
	// Ignore the "not found" error, we just want to test connectivity
	if err != nil && !strings.Contains(err.Error(), "not found") {
		logger.Warn("Failed to test connection", zap.Error(err))
	} else {
		logger.Info("Successfully connected to DKNet node")
	}

	// Create MCP server using the correct API
	s := server.NewMCPServer(
		"DKNet TSS MCP Client",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Register TSS tools
	if err := registerTSSTools(s, tssClient); err != nil {
		return fmt.Errorf("failed to register TSS tools: %w", err)
	}

	logger.Info("DKNet MCP Server ready - connect your LLM client via stdio")

	// Start the stdio server - this is the correct way
	return server.ServeStdio(s)
}

func contextWithAuth(ctx context.Context) context.Context {
	if jwtToken != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+jwtToken)
	}
	if nodeID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-node-id", nodeID)
	}
	return ctx
}

func registerTSSTools(s *server.MCPServer, tssClient tssv1.TSSServiceClient) error {
	// Register keygen tool
	keygenTool := mcp.NewTool("tss_keygen",
		mcp.WithDescription("Generate a new distributed threshold signature key using DKNet cluster"),
		mcp.WithNumber("threshold",
			mcp.Required(),
			mcp.Description("Minimum number of parties required to sign (t in t-of-n)"),
		),
		mcp.WithNumber("parties",
			mcp.Required(),
			mcp.Description("Total number of parties in the key generation (n in t-of-n)"),
		),
		mcp.WithString("participants",
			mcp.Required(),
			mcp.Description("Comma-separated list of peer IDs that should participate in key generation"),
		),
		mcp.WithString("operation_id",
			mcp.Description("Optional operation ID for idempotency"),
		),
	)

	s.AddTool(keygenTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Type assert arguments to map[string]interface{}
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments format"), nil
		}
		
		threshold, ok := args["threshold"].(float64)
		if !ok {
			return mcp.NewToolResultError("threshold must be a number"), nil
		}

		parties, ok := args["parties"].(float64)
		if !ok {
			return mcp.NewToolResultError("parties must be a number"), nil
		}

		participantsStr, ok := args["participants"].(string)
		if !ok {
			return mcp.NewToolResultError("participants must be a string"), nil
		}

		// Parse comma-separated participants
		participants := strings.Split(participantsStr, ",")
		for i := range participants {
			participants[i] = strings.TrimSpace(participants[i])
		}

		operationID := ""
		if opID, exists := args["operation_id"]; exists {
			if opIDStr, ok := opID.(string); ok {
				operationID = opIDStr
			}
		}

		// Validate parameters
		if int(threshold) > int(parties) {
			return mcp.NewToolResultError("threshold cannot be greater than total parties"), nil
		}

		if len(participants) != int(parties) {
			return mcp.NewToolResultError("number of participants must match total parties"), nil
		}

		// Start keygen operation via gRPC
		authCtx := contextWithAuth(ctx)
		resp, err := tssClient.StartKeygen(authCtx, &tssv1.StartKeygenRequest{
			OperationId:  operationID,
			Threshold:    int32(threshold),
			Parties:      int32(parties),
			Participants: participants,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to start keygen: %v", err)), nil
		}

		logger.Info("Keygen operation started",
			zap.String("operation_id", resp.OperationId))

		// Wait for operation to complete
		result, err := waitForOperationCompletion(authCtx, tssClient, resp.OperationId, 10*time.Minute)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Keygen operation failed: %v", err)), nil
		}

		response := fmt.Sprintf(`✅ Key generation completed successfully!

**Operation Details:**
- Operation ID: %s
- Status: %s
- Threshold: %d of %d
- Participants: %s
- Created: %s

**Generated Key:**
- Key ID: %s
- Public Key: %s

The distributed key has been securely generated and stored across the DKNet cluster.`,
			result.OperationId,
			result.Status.String(),
			int(threshold),
			int(parties),
			strings.Join(participants, ", "),
			result.CreatedAt.AsTime().Format(time.RFC3339),
			extractKeyID(result),
			extractPublicKey(result),
		)

		return mcp.NewToolResultText(response), nil
	})

	// Register signing tool
	signTool := mcp.NewTool("tss_sign",
		mcp.WithDescription("Sign a message using a distributed threshold signature key via DKNet cluster"),
		mcp.WithString("message",
			mcp.Required(),
			mcp.Description("Message to sign (plain text or hex)"),
		),
		mcp.WithString("key_id",
			mcp.Required(),
			mcp.Description("ID of the key to use for signing"),
		),
		mcp.WithString("participants",
			mcp.Required(),
			mcp.Description("Comma-separated list of peer IDs for signing"),
		),
		mcp.WithString("operation_id",
			mcp.Description("Optional operation ID"),
		),
		mcp.WithString("message_format",
			mcp.Description("Message format: 'text' or 'hex' (default: text)"),
		),
	)

	s.AddTool(signTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Type assert arguments to map[string]interface{}
		args, ok := req.Params.Arguments.(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("invalid arguments format"), nil
		}

		messageStr, ok := args["message"].(string)
		if !ok {
			return mcp.NewToolResultError("message must be a string"), nil
		}

		keyID, ok := args["key_id"].(string)
		if !ok {
			return mcp.NewToolResultError("key_id must be a string"), nil
		}

		participantsStr, ok := args["participants"].(string)
		if !ok {
			return mcp.NewToolResultError("participants must be a string"), nil
		}

		// Parse comma-separated participants
		participants := strings.Split(participantsStr, ",")
		for i := range participants {
			participants[i] = strings.TrimSpace(participants[i])
		}

		operationID := ""
		if opID, exists := args["operation_id"]; exists {
			if opIDStr, ok := opID.(string); ok {
				operationID = opIDStr
			}
		}

		messageFormat := "text"
		if format, exists := args["message_format"]; exists {
			if formatStr, ok := format.(string); ok {
				messageFormat = formatStr
			}
		}

		// Convert message to bytes based on format
		var messageBytes []byte

		switch messageFormat {
		case "hex":
			var decodeErr error
			messageBytes, decodeErr = hex.DecodeString(messageStr)
			if decodeErr != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid hex message: %v", decodeErr)), nil
			}
		case "text":
			messageBytes = []byte(messageStr)
		default:
			return mcp.NewToolResultError("message_format must be 'text' or 'hex'"), nil
		}

		// Start signing operation via gRPC
		authCtx := contextWithAuth(ctx)
		resp, err := tssClient.StartSigning(authCtx, &tssv1.StartSigningRequest{
			OperationId:  operationID,
			Message:      messageBytes,
			KeyId:        keyID,
			Participants: participants,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to start signing: %v", err)), nil
		}

		logger.Info("Signing operation started",
			zap.String("operation_id", resp.OperationId),
			zap.String("key_id", keyID))

		// Wait for operation to complete
		result, err := waitForOperationCompletion(authCtx, tssClient, resp.OperationId, 5*time.Minute)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Signing operation failed: %v", err)), nil
		}

		response := fmt.Sprintf(`✅ Message signed successfully!

**Operation Details:**
- Operation ID: %s
- Status: %s
- Key ID: %s
- Participants: %s
- Created: %s

**Message:**
- Original: %s
- Format: %s
- Hex: %s

**Signature:**
- R: %s
- S: %s
- V: %d

The message has been successfully signed using the distributed threshold signature scheme.`,
			result.OperationId,
			result.Status.String(),
			keyID,
			strings.Join(participants, ", "),
			result.CreatedAt.AsTime().Format(time.RFC3339),
			messageStr,
			messageFormat,
			hex.EncodeToString(messageBytes),
			extractSignatureR(result),
			extractSignatureS(result),
			extractRecoveryID(result),
		)

		return mcp.NewToolResultText(response), nil
	})

	return nil
}

// Helper function to wait for operation completion
func waitForOperationCompletion(ctx context.Context, tssClient tssv1.TSSServiceClient, operationID string, timeout time.Duration) (*tssv1.GetOperationResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	logger.Info("Waiting for operation completion",
		zap.String("operation_id", operationID),
		zap.Duration("timeout", timeout))

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("operation timed out after %v", timeout)
		case <-ticker.C:
			resp, err := tssClient.GetOperation(ctx, &tssv1.GetOperationRequest{
				OperationId: operationID,
			})
			if err != nil {
				logger.Debug("Failed to get operation status", zap.Error(err))
				continue
			}

			logger.Debug("Operation status check",
				zap.String("operation_id", operationID),
				zap.String("status", resp.Status.String()))

			switch resp.Status {
			case tssv1.OperationStatus_OPERATION_STATUS_COMPLETED:
				logger.Info("Operation completed successfully",
					zap.String("operation_id", operationID))
				return resp, nil
			case tssv1.OperationStatus_OPERATION_STATUS_FAILED:
				return nil, fmt.Errorf("operation failed")
			case tssv1.OperationStatus_OPERATION_STATUS_CANCELED:
				return nil, fmt.Errorf("operation was cancelled")
			}
		}
	}
}

// Helper functions to extract data from operation results
func extractKeyID(resp *tssv1.GetOperationResponse) string {
	if result := resp.GetKeygenResult(); result != nil {
		return result.KeyId
	}
	return "N/A"
}

func extractPublicKey(resp *tssv1.GetOperationResponse) string {
	if result := resp.GetKeygenResult(); result != nil {
		return result.PublicKey
	}
	return "N/A"
}

func extractSignatureR(resp *tssv1.GetOperationResponse) string {
	if result := resp.GetSigningResult(); result != nil {
		return result.R
	}
	return "N/A"
}

func extractSignatureS(resp *tssv1.GetOperationResponse) string {
	if result := resp.GetSigningResult(); result != nil {
		return result.S
	}
	return "N/A"
}

func extractRecoveryID(resp *tssv1.GetOperationResponse) int {
	if result := resp.GetSigningResult(); result != nil {
		return int(result.V)
	}
	return 0
} 