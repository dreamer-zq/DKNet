package e2e

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	tssv1 "github.com/dreamer-zq/DKNet/proto/tss/v1"
)

// Node represents a DKNet node configuration for testing.
type Node struct {
	Name    string
	PeerID  string
	Address string
	Client  tssv1.TSSServiceClient
	Conn    *grpc.ClientConn
}

// jwtCreds provides JWT-based credentials for gRPC calls.
type jwtCreds struct {
	token string
}

func newJWTCreds() (credentials.PerRPCCredentials, error) {
	secret := "dknet-test-jwt-secret-key-2024"
	issuer := "dknet-test"

	claims := jwt.MapClaims{
		"sub":   "test-user",
		"iss":   issuer,
		"iat":   time.Now().Unix(),
		"roles": []string{"admin", "operator"},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign JWT: %w", err)
	}

	

	return &jwtCreds{token: tokenString}, nil
}

func (c *jwtCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + c.token,
	}, nil
}

func (c *jwtCreds) RequireTransportSecurity() bool {
	return false // We are using insecure credentials for testing
}

// TestConfig holds the configuration for the E2E test suite.
var TestConfig = map[string]*Node{
	"node1": {
		Name:    "node1",
		PeerID:  "16Uiu2HAmUx7q8FPDyEs5pFMm3CPa86oUi1u7539pFBUaZavwMwZ8",
		Address: "localhost:19095",
	},
	"node2": {
		Name:    "node2",
		PeerID:  "16Uiu2HAmQDjiQMDSJWYrZ8e6ZKvYb4BT2cGSzU1wSkUzgiDeytEs",
		Address: "localhost:19096",
	},
	"node3": {
		Name:    "node3",
		PeerID:  "16Uiu2HAmM4qA4SBRDL3mtUwfqZJEXeS1xuVjrKedBrCS7r1phcQ6",
		Address: "localhost:19097",
	},
}

// setupClients connects to all nodes and creates gRPC clients.
func setupClients(t *testing.T) {
	creds, err := newJWTCreds()
	if err != nil {
		t.Fatalf("Failed to create JWT credentials: %v", err)
	}

	for _, node := range TestConfig {
		conn, err := grpc.NewClient(node.Address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithPerRPCCredentials(creds),
		)
		if err != nil {
			t.Fatalf("Failed to connect to %s: %v", node.Name, err)
		}
		node.Conn = conn
		node.Client = tssv1.NewTSSServiceClient(conn)
	}
}

// teardownClients closes all gRPC connections.
func teardownClients(t *testing.T) {
	for _, node := range TestConfig {
		if node.Conn != nil {
			if err := node.Conn.Close(); err != nil {
				t.Logf("Failed to close connection to %s: %v", node.Name, err)
			}
		}
	}
}

// waitForOperation waits for a TSS operation to complete on a specific node.
func waitForOperation(t *testing.T, node *Node, opID string) *tssv1.GetOperationResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for operation %s on %s", opID, node.Name)
		default:
			resp, err := node.Client.GetOperation(ctx, &tssv1.GetOperationRequest{OperationId: opID})
			if err != nil {
				t.Fatalf("Failed to get operation status for %s on %s: %v", opID, node.Name, err)
			}

			switch resp.Status {
			case tssv1.OperationStatus_OPERATION_STATUS_COMPLETED:
				log.Printf("Operation %s completed successfully on %s", opID, node.Name)
				return resp
			case tssv1.OperationStatus_OPERATION_STATUS_FAILED, tssv1.OperationStatus_OPERATION_STATUS_CANCELED:
				t.Fatalf("Operation %s on %s failed with status %s: %s", opID, node.Name, resp.Status, resp.GetError())
			case tssv1.OperationStatus_OPERATION_STATUS_PENDING, tssv1.OperationStatus_OPERATION_STATUS_IN_PROGRESS:
				// Continue waiting
				time.Sleep(2 * time.Second)
			default:
				t.Fatalf("Unknown operation status %s for op %s on %s", resp.Status, opID, node.Name)
			}
		}
	}
}

// findPubKeyInResponse searches for the public key in the operation response.
func findPubKeyInResponse(resp *tssv1.GetOperationResponse) (string, error) {
	if resp.GetResult() == nil {
		return "", fmt.Errorf("operation response has no result")
	}
	keygenResult, ok := resp.GetResult().(*tssv1.GetOperationResponse_KeygenResult)
	if !ok {
		return "", fmt.Errorf("result is not a KeygenResult, but %T", resp.GetResult())
	}

	pubKey := keygenResult.KeygenResult.GetPublicKey()
	if pubKey == "" {
		return "", fmt.Errorf("extracted public key is empty")
	}
	return pubKey, nil
}

// findSignatureInResponse searches for the signature in the operation response.
func findSignatureInResponse(resp *tssv1.GetOperationResponse) (string, error) {
	if resp.GetResult() == nil {
		return "", fmt.Errorf("operation response has no result")
	}
	signingResult, ok := resp.GetResult().(*tssv1.GetOperationResponse_SigningResult)
	if !ok {
		return "", fmt.Errorf("result is not a SigningResult, but %T", resp.GetResult())
	}

	sig := signingResult.SigningResult.GetSignature()
	if sig == "" {
		return "", fmt.Errorf("extracted signature is empty")
	}
	return sig, nil
}
