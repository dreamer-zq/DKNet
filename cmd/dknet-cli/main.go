package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"

	"github.com/dreamer-zq/DKNet/internal/api"
	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// Global configuration
var (
	// Global variables for connection management
	grpcConn   *grpc.ClientConn
	httpClient *http.Client
	tssClient  tssv1.TSSServiceClient

	// Command line flags
	serverAddr   string
	useGRPC      bool
	timeout      time.Duration
	outputFormat string

	// Authentication flags
	jwtToken string
)

var rootCmd = &cobra.Command{
	Use:   "dknet-cli",
	Short: "DKNet CLI - Command line client for DKNet TSS operations",
	Long: `DKNet CLI is a command line client for interacting with DKNet TSS servers.
	
It provides commands for key generation, signing, resharing, and other
threshold signature scheme operations through HTTP or gRPC APIs.

Authentication (optional):
  JWT authentication can be enabled on the server. When enabled, provide a token using:
  1. Command line flag: --token="your-jwt-token"
  2. Environment variable: export DKNET_JWT_TOKEN="your-jwt-token"
  
  If authentication is not enabled on the server, omit the token entirely.

Examples:
  # Without authentication
  dknet-cli keygen --threshold=2 --parties=3 --participants=node1,node2,node3
  
  # With token via flag
  dknet-cli --token="your-jwt-token" keygen --threshold=2 --parties=3 --participants=node1,node2,node3
  
  # With token via environment variable
  export DKNET_JWT_TOKEN="your-jwt-token"
  dknet-cli keygen --threshold=2 --parties=3 --participants=node1,node2,node3
  
  # Generate token on server (if auth is enabled)
  dknet generate-token --config=/path/to/config.yaml`,
	PersistentPreRunE: setupConnection,
	PersistentPostRun: cleanup,
}

func main() {
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:8080", "Server address (host:port)")
	rootCmd.PersistentFlags().BoolVarP(&useGRPC, "grpc", "g", false, "Use gRPC instead of HTTP")
	rootCmd.PersistentFlags().DurationVarP(&timeout, "timeout", "t", 30*time.Second, "Request timeout")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text|json)")

	// Authentication flags
	rootCmd.PersistentFlags().StringVar(&jwtToken, "token", "", "JWT token for authentication (can also use DKNET_JWT_TOKEN env var)")

	rootCmd.AddCommand(
		createKeygenCommand(),
		createSignCommand(),
		createReshareCommand(),
		createGetOperationCommand(),
	)

	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func outputJSON(data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(jsonData))
	return nil
}

func createKeygenCommand() *cobra.Command {
	var threshold int
	var participants []string

	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Start a key generation operation",
		Long:  "Start a new threshold key generation operation with specified parameters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if threshold < 0 {
				return fmt.Errorf("threshold must be non-negative")
			}
			if len(participants) == 0 {
				return fmt.Errorf("participants list cannot be empty")
			}
			if threshold >= len(participants) {
				return fmt.Errorf("threshold must be less than total participants (t+1 <= n required)")
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return keygenGRPC(ctx, threshold, participants)
			}
			return keygenHTTP(ctx, threshold, participants)
		},
	}

	cmd.Flags().IntVarP(&threshold, "threshold", "r", 0,
		"Fault tolerance threshold (t in (t+1)-of-n scheme). Max number of parties that can fail. Minimum signers required = t+1 (required)")
	cmd.Flags().StringSliceVarP(&participants, "participants", "P", nil, "List of participant IDs (required)")

	if err := cmd.MarkFlagRequired("threshold"); err != nil {
		panic(fmt.Sprintf("Failed to mark threshold flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("participants"); err != nil {
		panic(fmt.Sprintf("Failed to mark participants flag as required: %v", err))
	}

	return cmd
}

func createSignCommand() *cobra.Command {
	var message, keyID string
	var messageHex bool
	var participants []string

	cmd := &cobra.Command{
		Use:   "sign",
		Short: "Start a signing operation",
		Long:  "Start a new signing operation for the specified message and key.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if message == "" {
				return fmt.Errorf("message is required")
			}
			if keyID == "" {
				return fmt.Errorf("key-id is required")
			}
			if len(participants) == 0 {
				return fmt.Errorf("participants list cannot be empty")
			}

			var messageBytes []byte
			var err error

			if messageHex {
				messageBytes, err = hex.DecodeString(message)
				if err != nil {
					return fmt.Errorf("invalid hex message: %w", err)
				}
			} else {
				messageBytes = []byte(message)
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return signGRPC(ctx, messageBytes, keyID, participants)
			}
			return signHTTP(ctx, messageBytes, keyID, participants)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Message to sign (required)")
	cmd.Flags().StringVarP(&keyID, "key-id", "k", "", "Key ID to use for signing (required)")
	cmd.Flags().BoolVar(&messageHex, "hex", false, "Treat message as hex string")
	cmd.Flags().StringSliceVarP(&participants, "participants", "P", nil, "List of participant IDs (required)")

	if err := cmd.MarkFlagRequired("message"); err != nil {
		panic(fmt.Sprintf("Failed to mark message flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("key-id"); err != nil {
		panic(fmt.Sprintf("Failed to mark key-id flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("participants"); err != nil {
		panic(fmt.Sprintf("Failed to mark participants flag as required: %v", err))
	}

	return cmd
}

func createReshareCommand() *cobra.Command {
	var keyID string
	var newThreshold int
	var oldParticipants, newParticipants []string

	cmd := &cobra.Command{
		Use:   "reshare",
		Short: "Start a key resharing operation",
		Long:  "Start a new key resharing operation to change the threshold or participants.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyID == "" {
				return fmt.Errorf("key-id is required")
			}
			if newThreshold < 0 {
				return fmt.Errorf("new-threshold must be a non-negative integer")
			}
			if len(oldParticipants) == 0 {
				return fmt.Errorf("old-participants list cannot be empty")
			}
			if len(newParticipants) == 0 {
				return fmt.Errorf("new-participants list cannot be empty")
			}
			if newThreshold >= len(newParticipants) {
				return fmt.Errorf("new-threshold (%d) must be less than number of new-participants (%d)", newThreshold, len(newParticipants))
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return reshareGRPC(ctx, keyID, newThreshold, oldParticipants, newParticipants)
			}
			return reshareHTTP(ctx, keyID, newThreshold, oldParticipants, newParticipants)
		},
	}

	cmd.Flags().StringVarP(&keyID, "key-id", "k", "", "Key ID to reshare (required)")
	cmd.Flags().IntVar(&newThreshold, "new-threshold", 0,
		"New fault tolerance threshold (t in (t+1)-of-n scheme). Max number of parties that can fail. Minimum signers required = t+1 (required)")
	cmd.Flags().StringSliceVar(&oldParticipants, "old-participants", nil, "List of old participant IDs (required)")
	cmd.Flags().StringSliceVar(&newParticipants, "new-participants", nil, "List of new participant IDs (required)")

	if err := cmd.MarkFlagRequired("key-id"); err != nil {
		panic(fmt.Sprintf("Failed to mark key-id flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("new-threshold"); err != nil {
		panic(fmt.Sprintf("Failed to mark new-threshold flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("old-participants"); err != nil {
		panic(fmt.Sprintf("Failed to mark old-participants flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("new-participants"); err != nil {
		panic(fmt.Sprintf("Failed to mark new-participants flag as required: %v", err))
	}

	return cmd
}

func createGetOperationCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "operation <operation-id>",
		Short: "Get operation status and result",
		Long:  "Retrieve the status and result of a specific operation by its ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			operationID := args[0]

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return getOperationGRPC(ctx, operationID)
			}
			return getOperationHTTP(ctx, operationID)
		},
	}

	return cmd
}

// gRPC implementations
func keygenGRPC(ctx context.Context, threshold int, participants []string) error {
	// Add authentication to context
	ctx = addAuthToContext(ctx)

	req := &tssv1.StartKeygenRequest{
		Threshold:    int32(threshold),
		Participants: participants,
	}

	resp, err := tssClient.StartKeygen(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start keygen: %w", err)
	}

	return outputStartKeygenResponse(resp)
}

func signGRPC(ctx context.Context, message []byte, keyID string, participants []string) error {
	// Add authentication to context
	ctx = addAuthToContext(ctx)

	req := &tssv1.StartSigningRequest{
		Message:      message,
		KeyId:        keyID,
		Participants: participants,
	}

	resp, err := tssClient.StartSigning(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start signing: %w", err)
	}

	return outputStartSigningResponse(resp)
}

func reshareGRPC(ctx context.Context, keyID string, newThreshold int, oldParticipants, newParticipants []string) error {
	// Add authentication to context
	ctx = addAuthToContext(ctx)

	req := &tssv1.StartResharingRequest{
		KeyId:           keyID,
		NewThreshold:    int32(newThreshold),
		OldParticipants: oldParticipants,
		NewParticipants: newParticipants,
	}

	resp, err := tssClient.StartResharing(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start resharing: %w", err)
	}

	return outputStartResharingResponse(resp)
}

func getOperationGRPC(ctx context.Context, operationID string) error {
	// Add authentication to context
	ctx = addAuthToContext(ctx)

	req := &tssv1.GetOperationRequest{
		OperationId: operationID,
	}

	resp, err := tssClient.GetOperation(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get operation: %w", err)
	}

	return outputGetOperationResponse(resp)
}

// HTTP implementations
func keygenHTTP(ctx context.Context, threshold int, participants []string) error {
	req := &tssv1.StartKeygenRequest{
		Threshold:    int32(threshold),
		Participants: participants,
	}

	resp, err := makeHTTPRequest(ctx, "POST", api.FullKeygenPath, req)
	if err != nil {
		return err
	}

	var opResp tssv1.StartKeygenResponse
	if err := json.Unmarshal(resp, &opResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return outputStartKeygenResponse(&opResp)
}

func signHTTP(ctx context.Context, message []byte, keyID string, participants []string) error {
	req := &tssv1.StartSigningRequest{
		Message:      message,
		KeyId:        keyID,
		Participants: participants,
	}

	resp, err := makeHTTPRequest(ctx, "POST", api.FullSignPath, req)
	if err != nil {
		return err
	}

	var opResp tssv1.StartSigningResponse
	if err := json.Unmarshal(resp, &opResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return outputStartSigningResponse(&opResp)
}

func reshareHTTP(ctx context.Context, keyID string, newThreshold int, oldParticipants, newParticipants []string) error {
	req := &tssv1.StartResharingRequest{
		KeyId:           keyID,
		NewThreshold:    int32(newThreshold),
		OldParticipants: oldParticipants,
		NewParticipants: newParticipants,
	}

	resp, err := makeHTTPRequest(ctx, "POST", api.FullResharePath, req)
	if err != nil {
		return err
	}

	var opResp tssv1.StartResharingResponse
	if err := json.Unmarshal(resp, &opResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return outputStartResharingResponse(&opResp)
}

func getOperationHTTP(ctx context.Context, operationID string) error {
	resp, err := makeHTTPRequest(ctx, "GET", api.GetOperationPath(operationID), nil)
	if err != nil {
		return err
	}

	// For now, let's output the raw JSON response since there's a format mismatch
	if outputFormat == "json" {
		var rawResp map[string]interface{}
		if err := json.Unmarshal(resp, &rawResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		return outputJSON(rawResp)
	}

	// Parse the raw response for text output
	var rawResp map[string]interface{}
	if err := json.Unmarshal(resp, &rawResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return outputRawOperationResponse(rawResp)
}
