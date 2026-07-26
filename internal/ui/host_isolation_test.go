package ui

import (
	"testing"

	"github.com/opus-domini/sentinel/internal/testenv"
)

func TestMain(m *testing.M) {
	hostname = func() (string, error) { return "sentinel-test-host", nil }
	testenv.Run(m)
}
