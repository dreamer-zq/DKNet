package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

const (
	outputFormatJSON = "json"
	unknownValue     = "Unknown"
)

func setupConnection(cmd *cobra.Command, args []string) error {
	// Check for JWT token from environment variable if not provided via flag
	if jwtToken == "" {
		if envToken := os.Getenv("DKNET_JWT_TOKEN"); envToken != "" {
			jwtToken = envToken
		}
	}

	if useGRPC {
		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return fmt.Errorf("failed to connect to gRPC server: %w", err)
		}
		grpcConn = conn

		tssClient = tssv1.NewTSSServiceClient(grpcConn)
		return nil
	}

	// Setup HTTP client
	httpClient = &http.Client{
		Timeout: timeout,
	}

	// Adjust server address for HTTP if needed
	if !strings.HasPrefix(serverAddr, "http://") && !strings.HasPrefix(serverAddr, "https://") {
		serverAddr = "http://" + serverAddr
	}

	return nil
}

func makeHTTPRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	url := serverAddr + path
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add JWT authentication if token is provided
	if jwtToken != "" {
		req.Header.Set("Authorization", "Bearer "+jwtToken)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			// Log the error but don't fail the request
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", closeErr)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errorResp map[string]interface{}
		if json.Unmarshal(respBody, &errorResp) == nil {
			if errorMsg, ok := errorResp["error"].(string); ok {
				return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errorMsg)
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func cleanup(_ *cobra.Command, _ []string) {
	if grpcConn != nil {
		_ = grpcConn.Close()
	}
}

// addAuthToContext adds JWT authentication to gRPC context
func addAuthToContext(ctx context.Context) context.Context {
	if jwtToken != "" {
		md := metadata.Pairs("authorization", "Bearer "+jwtToken)
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

// Unified output functions
func outputStartKeygenResponse(resp *tssv1.StartKeygenResponse) error {
	if outputFormat == outputFormatJSON {
		return outputJSON(resp)
	}

	fmt.Printf("✅ Operation started successfully\n")
	fmt.Printf("Operation ID: %s\n", resp.OperationId)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Created At: %s\n", resp.CreatedAt.AsTime().Format(time.RFC3339))

	return nil
}

func outputStartSigningResponse(resp *tssv1.StartSigningResponse) error {
	if outputFormat == outputFormatJSON {
		return outputJSON(resp)
	}

	fmt.Printf("✅ Operation started successfully\n")
	fmt.Printf("Operation ID: %s\n", resp.OperationId)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Created At: %s\n", resp.CreatedAt.AsTime().Format(time.RFC3339))

	return nil
}

func outputStartResharingResponse(resp *tssv1.StartResharingResponse) error {
	if outputFormat == outputFormatJSON {
		return outputJSON(resp)
	}

	fmt.Printf("✅ Operation started successfully\n")
	fmt.Printf("Operation ID: %s\n", resp.OperationId)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Created At: %s\n", resp.CreatedAt.AsTime().Format(time.RFC3339))

	return nil
}

func outputGetOperationResponse(resp *tssv1.GetOperationResponse) error {
	if outputFormat == outputFormatJSON {
		return outputJSON(resp)
	}

	// Text format output
	fmt.Printf("📋 Operation Details\n")
	fmt.Printf("Operation ID: %s\n", resp.OperationId)
	fmt.Printf("Type: %s\n", resp.Type)
	fmt.Printf("Session ID: %s\n", resp.SessionId)
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Participants: %s\n", strings.Join(resp.Participants, ", "))
	fmt.Printf("Created At: %s\n", resp.CreatedAt.AsTime().Format(time.RFC3339))

	if resp.CompletedAt != nil {
		fmt.Printf("Completed At: %s\n", resp.CompletedAt.AsTime().Format(time.RFC3339))
	}

	if resp.Error != nil {
		fmt.Printf("❌ Error: %s\n", *resp.Error)
	}

	if resp.Result != nil {
		fmt.Printf("🎯 Result:\n")
		switch result := resp.Result.(type) {
		case *tssv1.GetOperationResponse_KeygenResult:
			fmt.Printf("  Public Key: %s\n", result.KeygenResult.PublicKey)
			fmt.Printf("  Key ID: %s\n", result.KeygenResult.KeyId)
		case *tssv1.GetOperationResponse_SigningResult:
			fmt.Printf("  Signature: %s\n", result.SigningResult.Signature)
			fmt.Printf("  R: %s\n", result.SigningResult.R)
			fmt.Printf("  S: %s\n", result.SigningResult.S)
		case *tssv1.GetOperationResponse_ResharingResult:
			fmt.Printf("  New Public Key: %s\n", result.ResharingResult.PublicKey)
			fmt.Printf("  New Key ID: %s\n", result.ResharingResult.KeyId)
		}
	}

	if resp.Request != nil {
		fmt.Printf("📝 Original Request:\n")
		switch request := resp.Request.(type) {
		case *tssv1.GetOperationResponse_KeygenRequest:
			fmt.Printf("  Threshold: %d\n", request.KeygenRequest.Threshold)
			fmt.Printf("  Participants: %s\n", strings.Join(request.KeygenRequest.Participants, ", "))
			fmt.Printf("  Parties: %d\n", len(request.KeygenRequest.Participants))
		case *tssv1.GetOperationResponse_SigningRequest:
			fmt.Printf("  Key ID: %s\n", request.SigningRequest.KeyId)
			fmt.Printf("  Message: %x\n", request.SigningRequest.Message)
			fmt.Printf("  Participants: %s\n", strings.Join(request.SigningRequest.Participants, ", "))
		case *tssv1.GetOperationResponse_ResharingRequest:
			fmt.Printf("  Key ID: %s\n", request.ResharingRequest.KeyId)
			fmt.Printf("  New Threshold: %d\n", request.ResharingRequest.NewThreshold)
			fmt.Printf("  Old Participants: %s\n", strings.Join(request.ResharingRequest.OldParticipants, ", "))
			fmt.Printf("  New Participants: %s\n", strings.Join(request.ResharingRequest.NewParticipants, ", "))
			fmt.Printf("  New Parties: %d\n", len(request.ResharingRequest.NewParticipants))
		}
	}

	return nil
}

// outputRawOperationResponse outputs operation response from raw JSON
func outputRawOperationResponse(resp map[string]interface{}) error {
	fmt.Printf("📋 Operation Details\n")

	if operationID, ok := resp["operation_id"].(string); ok {
		fmt.Printf("Operation ID: %s\n", operationID)
	}

	if opType, ok := resp["type"].(float64); ok {
		typeStr := unknownValue
		switch int(opType) {
		case 1:
			typeStr = "KEYGEN"
		case 2:
			typeStr = "SIGNING"
		case 3:
			typeStr = "RESHARING"
		}
		fmt.Printf("Type: %s\n", typeStr)
	}

	if sessionID, ok := resp["session_id"].(string); ok {
		fmt.Printf("Session ID: %s\n", sessionID)
	}

	if status, ok := resp["status"].(float64); ok {
		statusStr := unknownValue
		switch int(status) {
		case 1:
			statusStr = "PENDING"
		case 2:
			statusStr = "IN_PROGRESS"
		case 3:
			statusStr = "COMPLETED"
		case 4:
			statusStr = "FAILED"
		case 5:
			statusStr = "CANCELED"
		}
		fmt.Printf("Status: %s\n", statusStr)
	}

	if participants, ok := resp["participants"].([]interface{}); ok {
		var participantStrs []string
		for _, p := range participants {
			if pStr, ok := p.(string); ok {
				participantStrs = append(participantStrs, pStr)
			}
		}
		fmt.Printf("Participants: %s\n", strings.Join(participantStrs, ", "))
	}

	if createdAt, ok := resp["created_at"].(map[string]interface{}); ok {
		if seconds, ok := createdAt["seconds"].(float64); ok {
			t := time.Unix(int64(seconds), 0)
			fmt.Printf("Created At: %s\n", t.Format(time.RFC3339))
		}
	}

	if completedAt, ok := resp["completed_at"].(map[string]interface{}); ok {
		if seconds, ok := completedAt["seconds"].(float64); ok {
			t := time.Unix(int64(seconds), 0)
			fmt.Printf("Completed At: %s\n", t.Format(time.RFC3339))
		}
	}

	if errorMsg, ok := resp["error"].(string); ok && errorMsg != "" {
		fmt.Printf("❌ Error: %s\n", errorMsg)
	}

	// Handle results
	if result, ok := resp["Result"]; ok && result != nil {
		fmt.Printf("🎯 Result:\n")
		if resultMap, ok := result.(map[string]interface{}); ok {
			if keygenResult, ok := resultMap["KeygenResult"].(map[string]interface{}); ok {
				if publicKey, ok := keygenResult["public_key"].(string); ok {
					fmt.Printf("  Public Key: %s\n", publicKey)
				}
				if keyID, ok := keygenResult["key_id"].(string); ok {
					fmt.Printf("  Key ID: %s\n", keyID)
				}
			}
			if signingResult, ok := resultMap["SigningResult"].(map[string]interface{}); ok {
				if signature, ok := signingResult["signature"].(string); ok {
					fmt.Printf("  Signature: %s\n", signature)
				}
				if r, ok := signingResult["r"].(string); ok {
					fmt.Printf("  R: %s\n", r)
				}
				if s, ok := signingResult["s"].(string); ok {
					fmt.Printf("  S: %s\n", s)
				}
			}
		}
	}

	// Handle original request
	if request, ok := resp["Request"]; ok && request != nil {
		fmt.Printf("📝 Original Request:\n")
		if requestMap, ok := request.(map[string]interface{}); ok {
			if keygenReq, ok := requestMap["KeygenRequest"].(map[string]interface{}); ok {
				if threshold, ok := keygenReq["threshold"].(float64); ok {
					fmt.Printf("  Threshold: %d\n", int(threshold))
				}
				if participants, ok := keygenReq["participants"].([]interface{}); ok {
					var participantStrs []string
					for _, p := range participants {
						if pStr, ok := p.(string); ok {
							participantStrs = append(participantStrs, pStr)
						}
					}
					fmt.Printf("  Participants: %s\n", strings.Join(participantStrs, ", "))
					fmt.Printf("  Parties: %d\n", len(participantStrs))
				}
			}
			if signingReq, ok := requestMap["SigningRequest"].(map[string]interface{}); ok {
				if keyID, ok := signingReq["key_id"].(string); ok {
					fmt.Printf("  Key ID: %s\n", keyID)
				}
				if message, ok := signingReq["message"].(string); ok {
					fmt.Printf("  Message: %s\n", message)
				}
			}
		}
	}

	return nil
}
