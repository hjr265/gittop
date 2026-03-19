package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds user preferences persisted across sessions.
type Config struct {
	ThemeName   string `toml:"theme"`        // "Default", "Grayscale", or "Custom"
	Truecolor   bool   `toml:"truecolor"`    // true = 24-bit, false = 256-color
	GraphSymbol string `toml:"graph_symbol"` // "braille" or "block"
	CustomTheme *Theme `toml:"custom_theme,omitempty"`
}

func defaultConfig() Config {
	return Config{
		ThemeName:   "default",
		GraphSymbol: "braille",
	}
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gittop", "config.toml"), nil
}

// LoadConfig reads the config file, returning defaults if it doesn't exist.
func LoadConfig() Config {
	cfg := defaultConfig()
	path, err := configPath()
	if err != nil {
		return cfg
	}
	_, err = toml.DecodeFile(path, &cfg)
	if err != nil {
		return cfg
	}
	return cfg
}

// SaveConfig writes the config to disk.
func SaveConfig(cfg Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// GraphSymbolValue returns the GraphSymbol enum value from the config string.
func (cfg Config) GraphSymbolValue() GraphSymbol {
	if cfg.GraphSymbol == "block" {
		return GraphBlock
	}
	return GraphBraille
}

// ToConfig extracts current settings into a Config.
func (m model) ToConfig() Config {
	sym := "braille"
	if m.graphSymbol == GraphBlock {
		sym = "block"
	}
	return Config{
		ThemeName:   strings.ToLower(m.theme.Name),
		Truecolor:   m.truecolor,
		GraphSymbol: sym,
	}
}
