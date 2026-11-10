package server

import (
	"encoding/json"
	"log/slog"
	"os"
)

// warnIgnoredDevURL logs once when the config file still holds the removed
// server.web.dev-url. The file is only read, never rewritten.
func warnIgnoredDevURL(log *slog.Logger, configPath string) {
	if configPath == "" {
		return
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var raw struct {
		Server struct {
			Web map[string]json.RawMessage `json:"web"`
		} `json:"server"`
	}
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	if _, ok := raw.Server.Web["dev-url"]; ok {
		log.Warn("server.web.dev-url is ignored: Dev mode now runs its own Vite from the Dev checkout", "key", "server.web.dev-url", "config", configPath)
	}
}
