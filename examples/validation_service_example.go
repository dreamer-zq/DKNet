package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ValidationRequest represents the request from TSS node
type ValidationRequest struct {
	Message      string                 `json:"message"`      // hex encoded message
	KeyID        string                 `json:"key_id"`       // key ID being used
	Participants []string               `json:"participants"` // participant node IDs
	NodeID       string                 `json:"node_id"`      // requesting node ID
	Timestamp    int64                  `json:"timestamp"`    // request timestamp
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ValidationResponse represents the response to TSS node
type ValidationResponse struct {
	Approved bool                   `json:"approved"`         // whether to approve the signing
	Reason   string                 `json:"reason,omitempty"` // reason for approval/rejection
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Simple validation rules for demonstration
func validateSigningRequest(req *ValidationRequest) *ValidationResponse {
	// Decode the hex message to inspect content
	messageBytes, err := hex.DecodeString(req.Message)
	if err != nil {
		return &ValidationResponse{
			Approved: false,
			Reason:   fmt.Sprintf("Invalid hex message: %v", err),
		}
	}

	messageStr := string(messageBytes)
	log.Printf("Validating signing request: KeyID=%s, Message=%s, NodeID=%s", 
		req.KeyID, messageStr, req.NodeID)

	// Example validation rules:
	
	// 1. Reject empty messages
	if len(messageBytes) == 0 {
		return &ValidationResponse{
			Approved: false,
			Reason:   "Empty message not allowed",
		}
	}

	// 2. Reject messages that are too long (>1KB)
	if len(messageBytes) > 1024 {
		return &ValidationResponse{
			Approved: false,
			Reason:   "Message too long (max 1KB allowed)",
		}
	}

	// 3. Reject messages containing forbidden words
	forbiddenWords := []string{"malicious", "hack", "exploit"}
	for _, word := range forbiddenWords {
		if strings.Contains(strings.ToLower(messageStr), word) {
			return &ValidationResponse{
				Approved: false,
				Reason:   fmt.Sprintf("Message contains forbidden word: %s", word),
			}
		}
	}

	// 4. Only allow certain key IDs (whitelist)
	allowedKeyIDs := []string{
		"0xfa3cd17afd7e5d98d02fbad669adc46e7512bbb4", // example key ID
		"0x1234567890abcdef1234567890abcdef12345678", // another example
	}
	keyAllowed := false
	for _, allowedKey := range allowedKeyIDs {
		if strings.EqualFold(req.KeyID, allowedKey) {
			keyAllowed = true
			break
		}
	}
	if !keyAllowed {
		return &ValidationResponse{
			Approved: false,
			Reason:   fmt.Sprintf("Key ID %s is not in whitelist", req.KeyID),
		}
	}

	// 5. Check timestamp (reject requests older than 5 minutes)
	now := time.Now().Unix()
	if now-req.Timestamp > 300 { // 5 minutes
		return &ValidationResponse{
			Approved: false,
			Reason:   "Request timestamp too old (max 5 minutes)",
		}
	}

	// 6. Require minimum number of participants
	if len(req.Participants) < 2 {
		return &ValidationResponse{
			Approved: false,
			Reason:   "Minimum 2 participants required",
		}
	}

	// If all checks pass, approve the request
	return &ValidationResponse{
		Approved: true,
		Reason:   "All validation checks passed",
		Metadata: map[string]interface{}{
			"validated_at": time.Now().Unix(),
			"message_length": len(messageBytes),
		},
	}
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req ValidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Log the incoming request
	log.Printf("Received validation request from node %s for key %s", req.NodeID, req.KeyID)

	// Validate the request
	resp := validateSigningRequest(&req)

	// Log the decision
	if resp.Approved {
		log.Printf("APPROVED: %s", resp.Reason)
	} else {
		log.Printf("REJECTED: %s", resp.Reason)
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
		"service": "DKNet Validation Service",
		"version": "1.0.0",
	}); err != nil {
		log.Printf("Error encoding health response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

func main() {
	// Setup HTTP routes
	http.HandleFunc("/validate", validateHandler)
	http.HandleFunc("/health", healthHandler)

	// Start server
	port := ":8888"
	log.Printf("Starting DKNet Validation Service on port %s", port)
	log.Printf("Validation endpoint: http://localhost%s/validate", port)
	log.Printf("Health endpoint: http://localhost%s/health", port)
	
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
} 