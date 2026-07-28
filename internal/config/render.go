package config

// RenderTOML renders the complete canonical Sentinel config document.
func RenderTOML(cfg Config) []byte {
	return defaultConfigTOML(cfg)
}
