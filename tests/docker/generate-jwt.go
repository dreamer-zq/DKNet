package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func main() {
	// JWT secret used in test configurations
	secret := "dknet-test-jwt-secret-key-2024"
	issuer := "dknet-test"

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "test-user",
		"iss":   issuer,
		"iat":   time.Now().Unix(),
		"roles": []string{"admin", "operator"},
	})

	// Sign token with secret
	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		panic(err)
	}

	fmt.Println("Generated JWT Token for DKNet Testing:")
	fmt.Println("=====================================")
	fmt.Printf("Token: %s\n\n", tokenString)

	fmt.Println("Usage Examples:")
	fmt.Println("---------------")
	fmt.Println("HTTP:")
	fmt.Printf("curl -H \"Authorization: Bearer %s\" http://localhost:8080/api/v1/operations/test-id\n\n", tokenString)

	fmt.Println("gRPC:")
	fmt.Printf("grpcurl -H \"authorization: Bearer %s\" localhost:9090 tss.v1.TSSService/GetOperation\n\n", tokenString)

	fmt.Println("Token Claims:")
	fmt.Println("- Subject: test-user")
	fmt.Println("- Issuer: dknet-test")
	fmt.Println("- Roles: admin, operator")
	fmt.Printf("- Expires: %s\n", time.Unix(time.Now().Add(24*time.Hour).Unix(), 0).Format(time.RFC3339))
}
