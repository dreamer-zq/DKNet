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
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tss-client",
		Short: "DKNet Client - Command line tool for TSS operations",
		Long: `TSS Client is a command line tool for interacting with DKNet.
It supports both HTTP and gRPC protocols for calling TSS operations
like key generation, signing, resharing, and operation management.`,
		PersistentPreRunE: setupConnection,
		PersistentPostRun: cleanup,
	}

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:8080", "Server address (host:port)")
	rootCmd.PersistentFlags().BoolVarP(&useGRPC, "grpc", "g", false, "Use gRPC instead of HTTP")
	rootCmd.PersistentFlags().DurationVarP(&timeout, "timeout", "t", 30*time.Second, "Request timeout")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text|json)")

	// Add subcommands
	rootCmd.AddCommand(
		createKeygenCommand(),
		createSignCommand(),
		createReshareCommand(),
		createGetOperationCommand(),
		createNetworkInfoCommand(),
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
	var threshold, parties int
	var participants []string

	cmd := &cobra.Command{
		Use:   "keygen",
		Short: "Start a key generation operation",
		Long:  "Start a new threshold key generation operation with specified parameters.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if threshold <= 0 || parties <= 0 {
				return fmt.Errorf("threshold and parties must be positive integers")
			}
			if threshold > parties {
				return fmt.Errorf("threshold cannot be greater than parties")
			}
			if len(participants) != parties {
				return fmt.Errorf("number of participants (%d) must equal parties (%d)", len(participants), parties)
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return keygenGRPC(ctx, threshold, parties, participants)
			}
			return keygenHTTP(ctx, threshold, parties, participants)
		},
	}

	cmd.Flags().IntVarP(&threshold, "threshold", "r", 0, "Threshold number of parties required for signing (required)")
	cmd.Flags().IntVarP(&parties, "parties", "p", 0, "Total number of parties (required)")
	cmd.Flags().StringSliceVarP(&participants, "participants", "P", nil, "List of participant IDs (required)")

	if err := cmd.MarkFlagRequired("threshold"); err != nil {
		panic(fmt.Sprintf("Failed to mark threshold flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("parties"); err != nil {
		panic(fmt.Sprintf("Failed to mark parties flag as required: %v", err))
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
	var newThreshold, newParties int
	var oldParticipants, newParticipants []string

	cmd := &cobra.Command{
		Use:   "reshare",
		Short: "Start a key resharing operation",
		Long:  "Start a new key resharing operation to change the threshold or participants.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if keyID == "" {
				return fmt.Errorf("key-id is required")
			}
			if newThreshold <= 0 || newParties <= 0 {
				return fmt.Errorf("new-threshold and new-parties must be positive integers")
			}
			if newThreshold > newParties {
				return fmt.Errorf("new-threshold cannot be greater than new-parties")
			}
			if len(oldParticipants) == 0 {
				return fmt.Errorf("old-participants list cannot be empty")
			}
			if len(newParticipants) != newParties {
				return fmt.Errorf("number of new-participants (%d) must equal new-parties (%d)", len(newParticipants), newParties)
			}

			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return reshareGRPC(ctx, keyID, newThreshold, newParties, oldParticipants, newParticipants)
			}
			return reshareHTTP(ctx, keyID, newThreshold, newParties, oldParticipants, newParticipants)
		},
	}

	cmd.Flags().StringVarP(&keyID, "key-id", "k", "", "Key ID to reshare (required)")
	cmd.Flags().IntVar(&newThreshold, "new-threshold", 0, "New threshold number (required)")
	cmd.Flags().IntVar(&newParties, "new-parties", 0, "New total number of parties (required)")
	cmd.Flags().StringSliceVar(&oldParticipants, "old-participants", nil, "List of old participant IDs (required)")
	cmd.Flags().StringSliceVar(&newParticipants, "new-participants", nil, "List of new participant IDs (required)")

	if err := cmd.MarkFlagRequired("key-id"); err != nil {
		panic(fmt.Sprintf("Failed to mark key-id flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("new-threshold"); err != nil {
		panic(fmt.Sprintf("Failed to mark new-threshold flag as required: %v", err))
	}
	if err := cmd.MarkFlagRequired("new-parties"); err != nil {
		panic(fmt.Sprintf("Failed to mark new-parties flag as required: %v", err))
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

// createNetworkInfoCommand creates the network-info command
func createNetworkInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network-info",
		Short: "Get all node address mappings",
		Long:  "Query and display all known nodeID-peerID-moniker mappings in the network.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			if useGRPC {
				return getNetworkAddressesGRPC(ctx)
			}
			return getNetworkAddressesHTTP(ctx)
		},
	}
	return cmd
}

// getNetworkAddressesGRPC queries node mappings via gRPC
func getNetworkAddressesGRPC(ctx context.Context) error {
	resp, err := tssClient.GetNetworkAddresses(ctx, &tssv1.GetNetworkAddressesRequest{})
	if err != nil {
		return fmt.Errorf("failed to get network addresses (gRPC): %w", err)
	}
	return outputNetworkAddresses(resp)
}

// getNetworkAddressesHTTP queries node mappings via HTTP
func getNetworkAddressesHTTP(ctx context.Context) error {
	respBytes, err := makeHTTPRequest(ctx, "GET", "/api/v1/network/addresses", nil)
	if err != nil {
		return fmt.Errorf("failed to get network addresses (HTTP): %w", err)
	}
	var resp tssv1.GetNetworkAddressesResponse
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}
	return outputNetworkAddresses(&resp)
}

// gRPC implementations
func keygenGRPC(ctx context.Context, threshold, parties int, participants []string) error {
	req := &tssv1.StartKeygenRequest{
		Threshold:    int32(threshold),
		Parties:      int32(parties),
		Participants: participants,
	}

	resp, err := tssClient.StartKeygen(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start keygen: %w", err)
	}

	return outputStartKeygenResponse(resp)
}

func signGRPC(ctx context.Context, message []byte, keyID string, participants []string) error {
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

func reshareGRPC(ctx context.Context, keyID string, newThreshold, newParties int, oldParticipants, newParticipants []string) error {
	req := &tssv1.StartResharingRequest{
		KeyId:           keyID,
		NewThreshold:    int32(newThreshold),
		NewParties:      int32(newParties),
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
func keygenHTTP(ctx context.Context, threshold, parties int, participants []string) error {
	reqBody := map[string]interface{}{
		"threshold":    threshold,
		"parties":      parties,
		"participants": participants,
	}

	resp, err := makeHTTPRequest(ctx, "POST", "/api/v1/keygen", reqBody)
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
	reqBody := map[string]interface{}{
		"message":      message,
		"key_id":       keyID,
		"participants": participants,
	}

	resp, err := makeHTTPRequest(ctx, "POST", "/api/v1/sign", reqBody)
	if err != nil {
		return err
	}

	var opResp tssv1.StartSigningResponse
	if err := json.Unmarshal(resp, &opResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return outputStartSigningResponse(&opResp)
}

func reshareHTTP(ctx context.Context, keyID string, newThreshold, newParties int, oldParticipants, newParticipants []string) error {
	reqBody := map[string]interface{}{
		"key_id":           keyID,
		"new_threshold":    newThreshold,
		"new_parties":      newParties,
		"old_participants": oldParticipants,
		"new_participants": newParticipants,
	}

	resp, err := makeHTTPRequest(ctx, "POST", "/api/v1/reshare", reqBody)
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
	resp, err := makeHTTPRequest(ctx, "GET", "/operations/"+operationID, nil)
	if err != nil {
		return err
	}

	var opResp tssv1.GetOperationResponse
	if err := json.Unmarshal(resp, &opResp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return outputGetOperationResponse(&opResp)
}
