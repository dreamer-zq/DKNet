package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// ReadPassword reads a password from stdin without echoing
func ReadPassword(prompt string) (string, error) {
	fmt.Print(prompt)

	// Check if stdin is a terminal
	if term.IsTerminal(int(syscall.Stdin)) {
		// Read password without echo
		bytePassword, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return "", fmt.Errorf("failed to read password: %w", err)
		}
		fmt.Println() // Print newline after password input
		password := strings.TrimSpace(string(bytePassword))

		if len(password) == 0 {
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
	if len(password) == 0 {
		return "", fmt.Errorf("password cannot be empty")
	}

	return password, nil
}

// ReadPasswordWithConfirmation reads a password and asks for confirmation
func ReadPasswordWithConfirmation() (string, error) {
	password, err := ReadPassword("Enter encryption password: ")
	if err != nil {
		return "", err
	}

	if len(password) < 8 {
		return "", fmt.Errorf("password must be at least 8 characters long")
	}

	confirmation, err := ReadPassword("Confirm encryption password: ")
	if err != nil {
		return "", err
	}

	if password != confirmation {
		return "", fmt.Errorf("passwords do not match")
	}

	return password, nil
}

// ValidatePassword validates password strength
func ValidatePassword(password string) error {
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
