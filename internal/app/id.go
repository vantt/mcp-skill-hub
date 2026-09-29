package app

import (
	"crypto/rand"
	"encoding/hex"
)

// IDGenerator creates opaque identifiers for application-layer records.
type IDGenerator interface {
	New() (string, error)
}

// RandomIDGenerator creates opaque, cryptographically random identifiers.
type RandomIDGenerator struct{}

// New implements IDGenerator.
func (RandomIDGenerator) New() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
