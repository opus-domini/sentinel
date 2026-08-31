package notify

import (
	"testing"
	"time"

	"github.com/opus-domini/sentinel/internal/testenv"
)

func TestMain(m *testing.M) {
	// Collapse the retry backoff so the suite exercises the policy without
	// sleeping for it. Assigned before any test starts, so it stays race-free
	// with the parallel tests below.
	retryInterval = 5 * time.Millisecond
	testenv.Run(m)
}
