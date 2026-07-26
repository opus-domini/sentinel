package services

import (
	"testing"

	"github.com/opus-domini/sentinel/internal/testenv"
)

func TestMain(m *testing.M) {
	defaultHostname = func() (string, error) { return "sentinel-test-host", nil }
	defaultUID = func() int { return 1000 }
	testenv.Run(m, testenv.WithEmptyPath())
}
