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
	Mouse       bool   `toml:"mouse"`        // wheel scrolling and tab clicks
	// DefaultRange is the date range at startup: "3m", "6m", "1y", "2y",
	// "5y", or "all". Empty or unknown values mean "1y".
	DefaultRange string `toml:"default_range,omitempty"`
	CustomTheme  *Theme `toml:"custom_theme,omitempty"`
}

func defaultConfig() Config {
	return Config{
		ThemeName:   "default",
		GraphSymbol: "braille",
		Mouse:       true,
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

// ToConfig extracts current settings into a Config. Settings the options
// menu does not manage, such as the custom theme, keep their loaded values.
func (m model) ToConfig() Config {
	sym := "braille"
	if m.graphSymbol == GraphBlock {
		sym = "block"
	}
	cfg := m.config
	cfg.ThemeName = strings.ToLower(m.theme.Name)
	cfg.Truecolor = m.truecolor
	cfg.GraphSymbol = sym
	cfg.Mouse = m.mouse
	return cfg
}
