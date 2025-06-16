package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/cobra"

	"github.com/dreamer-zq/DKNet/internal/config"
)

const (
	neverExpires = "never"
)

// generateTokenCmd creates a command to generate JWT tokens for API authentication
func generateTokenCmd() *cobra.Command {
	var outputFormat string
	var userID string
	var roles []string
	var expiryHours int

	cmd := &cobra.Command{
		Use:   "generate-token",
		Short: "Generate JWT token for API authentication",
		Long: `Generate a JWT token for API authentication using the server's JWT configuration.
		
This command reads the JWT secret and issuer from the server configuration
and generates a token that can be used by clients to authenticate with the API.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load configuration to get JWT settings
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Check if JWT authentication is configured
			if !cfg.Security.APIAuth.Enabled {
				return fmt.Errorf("JWT authentication is not enabled in server configuration")
			}

			jwtConfig := cfg.Security.APIAuth
			if jwtConfig.JWTSecret == "" {
				return fmt.Errorf("JWT secret is not configured in server configuration")
			}

			// Set default values if not provided
			if userID == "" {
				userID = "admin-user"
			}
			if len(roles) == 0 {
				roles = []string{"admin", "operator"}
			}

			// Generate JWT token
			claims := jwt.MapClaims{
				"sub":   userID,
				"iss":   jwtConfig.JWTIssuer,
				"iat":   time.Now().Unix(),
				"roles": roles,
			}

			// Add expiration only if specified (0 means no expiration)
			if expiryHours > 0 {
				claims["exp"] = time.Now().Add(time.Duration(expiryHours) * time.Hour).Unix()
			}

			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

			tokenString, err := token.SignedString([]byte(jwtConfig.JWTSecret))
			if err != nil {
				return fmt.Errorf("failed to generate token: %w", err)
			}

			// Output the token
			if outputFormat == "json" {
				output := map[string]interface{}{
					"token":   tokenString,
					"user_id": userID,
					"issuer":  jwtConfig.JWTIssuer,
					"roles":   roles,
				}

				if expiryHours > 0 {
					output["expires_in"] = fmt.Sprintf("%dh", expiryHours)
					output["expires_at"] = time.Now().Add(time.Duration(expiryHours) * time.Hour).Format(time.RFC3339)
				} else {
					output["expires_in"] = neverExpires
					output["expires_at"] = neverExpires
				}
				jsonData, err := json.MarshalIndent(output, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal JSON: %w", err)
				}
				fmt.Println(string(jsonData))
			} else {
				fmt.Printf("🔑 JWT Token Generated Successfully\n")
				fmt.Printf("Token: %s\n", tokenString)
				fmt.Printf("User ID: %s\n", userID)
				fmt.Printf("Issuer: %s\n", jwtConfig.JWTIssuer)
				fmt.Printf("Roles: %v\n", roles)

				if expiryHours > 0 {
					fmt.Printf("Expires: %s (%dh)\n", time.Now().Add(time.Duration(expiryHours)*time.Hour).Format(time.RFC3339), expiryHours)
				} else {
					fmt.Printf("Expires: Never (permanent token)\n")
					fmt.Printf("⚠️  Warning: This is a permanent token. Store it securely!\n")
				}

				fmt.Printf("\nUsage with dknet-cli:\n")
				fmt.Printf("  dknet-cli --token=\"%s\" <command>\n", tokenString)
				fmt.Printf("\nUsage with curl:\n")
				fmt.Printf("  curl -H \"Authorization: Bearer %s\" <api-endpoint>\n", tokenString)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text|json)")
	cmd.Flags().StringVarP(&userID, "user", "u", "", "User ID for the token (default: admin-user)")
	cmd.Flags().StringSliceVarP(&roles, "roles", "r", nil, "Roles for the token (default: admin,operator)")
	cmd.Flags().IntVarP(&expiryHours, "expires", "e", 24, "Token expiry in hours (default: 24, use 0 for no expiration)")

	return cmd
}
