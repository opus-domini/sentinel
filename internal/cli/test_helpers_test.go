package cli

import (
	"path/filepath"

	"github.com/opus-domini/sentinel/internal/config"
)

func testCLIConfig(dataDir, token string) config.Config {
	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 4040
	cfg.Server.Token = token
	cfg.Storage.Path = filepath.Join(dataDir, "sentinel.db")
	cfg.Log.Path = filepath.Join(dataDir, "logs", "sentinel.log")
	return cfg
}

func testLoadConfig(dataDir, token string) func() (config.Config, string, error) {
	return func() (config.Config, string, error) {
		return testCLIConfig(dataDir, token), "", nil
	}
}
