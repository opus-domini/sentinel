package config

import (
	"os"
	"os/user"
	"testing"

	"github.com/opus-domini/sentinel/internal/testenv"
)

func TestMain(m *testing.M) {
	osCurrentUser = func() (*user.User, error) {
		return &user.User{Username: "sentinel-test", Uid: "1000", HomeDir: os.Getenv("HOME")}, nil
	}
	osGeteuid = func() int { return 1000 }
	testenv.Run(m)
}
