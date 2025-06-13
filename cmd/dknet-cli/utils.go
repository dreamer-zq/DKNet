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

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

const (
	outputFormatJSON = "json"
)

func setupConnection(cmd *cobra.Command, args []string) error {
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
			fmt.Printf("  Parties: %d\n", request.KeygenRequest.Parties)
			fmt.Printf("  Participants: %s\n", strings.Join(request.KeygenRequest.Participants, ", "))
		case *tssv1.GetOperationResponse_SigningRequest:
			fmt.Printf("  Key ID: %s\n", request.SigningRequest.KeyId)
			fmt.Printf("  Message: %x\n", request.SigningRequest.Message)
			fmt.Printf("  Participants: %s\n", strings.Join(request.SigningRequest.Participants, ", "))
		case *tssv1.GetOperationResponse_ResharingRequest:
			fmt.Printf("  Key ID: %s\n", request.ResharingRequest.KeyId)
			fmt.Printf("  New Threshold: %d\n", request.ResharingRequest.NewThreshold)
			fmt.Printf("  New Parties: %d\n", request.ResharingRequest.NewParties)
			fmt.Printf("  Old Participants: %s\n", strings.Join(request.ResharingRequest.OldParticipants, ", "))
			fmt.Printf("  New Participants: %s\n", strings.Join(request.ResharingRequest.NewParticipants, ", "))
		}
	}

	return nil
}

// outputNetworkAddresses pretty-prints the node mappings
func outputNetworkAddresses(resp *tssv1.GetNetworkAddressesResponse) error {
	if outputFormat == outputFormatJSON {
		return outputJSON(resp)
	}
	if len(resp.Mappings) == 0 {
		fmt.Println("No node mappings found.")
		return nil
	}
	fmt.Printf("%-12s %-48s %-20s %-30s\n", "NodeID", "PeerID", "Moniker", "Timestamp")
	for _, m := range resp.Mappings {
		ts := ""
		if m.Timestamp != nil {
			ts = m.Timestamp.AsTime().Format(time.RFC3339)
		}
		fmt.Printf("%-12s %-48s %-20s %-30s\n", m.NodeId, m.PeerId, m.Moniker, ts)
	}
	return nil
}