package report

import (
	"testing"

	"github.com/opus-domini/sentinel/internal/testenv"
)

func TestMain(m *testing.M) {
	osHostname = func() (string, error) { return "sentinel-test-host", nil }
	testenv.Run(m)
}
