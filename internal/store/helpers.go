package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

func randomID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("id-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
