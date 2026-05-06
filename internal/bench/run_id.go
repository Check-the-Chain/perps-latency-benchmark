package bench

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func NewRunID() string {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(entropy[:])
}
