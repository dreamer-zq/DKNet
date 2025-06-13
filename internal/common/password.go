package common

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// ReadPassword reads a password from stdin or environment variable
func ReadPassword() (string, error) {
	password, err := readPasswordFromEnv()
	if err == nil {
		if validationErr := validatePassword(password); validationErr != nil {
			return "", validationErr
		}
		return password, nil
	}

	// Fallback to interactive input
	password, err = readPasswordWithConfirmation()
	if err != nil {
		return "", err
	}

	if err := validatePassword(password); err != nil {
		return "", err
	}

	return password, nil
}

// ReadPasswordFromEnv reads password from environment variable only
func readPasswordFromEnv() (string, error) {
	// Try environment variable
	if password := os.Getenv("TSS_ENCRYPTION_PASSWORD"); password != "" {
		return password, nil
	}

	return "", fmt.Errorf("TSS_ENCRYPTION_PASSWORD environment variable not set")
}

// ReadPasswordWithConfirmation reads a password and asks for confirmation
func readPasswordWithConfirmation() (string, error) {
	password, err := readPassword("Enter encryption password: ")
	if err != nil {
		return "", err
	}

	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters long")
	}

	confirmation, err := readPassword("Confirm encryption password: ")
	if err != nil {
		return "", err
	}

	if password != confirmation {
		return "", fmt.Errorf("passwords do not match")
	}

	return password, nil
}

// readPassword reads a password from stdin without echoing
func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	// Check if stdin is a terminal
	if term.IsTerminal(syscall.Stdin) {
		// Read password without echo
		bytePassword, err := term.ReadPassword(syscall.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // Print newline after password input
		password := strings.TrimSpace(string(bytePassword))

		if password == "" {
			return "", fmt.Errorf("password cannot be empty")
		}

		return password, nil
	}
	// Fallback for non-terminal input (e.g., pipes, redirects)
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	password = strings.TrimSpace(password)
	if password == "" {
		return "", fmt.Errorf("password cannot be empty")
	}

	return password, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasDigit {
		missing = append(missing, "digit")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}

	if len(missing) > 0 {
		return fmt.Errorf("password must contain at least one: %s", strings.Join(missing, ", "))
	}

	return nil
}
