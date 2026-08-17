package survey

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewID creates a sortable, random survey identifier.
func NewID() (string, error) {
	randomPart := make([]byte, 6)
	if _, err := rand.Read(randomPart); err != nil {
		return "", fmt.Errorf("generate survey id: %w", err)
	}
	return "survey-" + time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(randomPart), nil
}
