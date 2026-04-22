package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	BaseDir string
}

type fileConfig struct {
	BaseDir string `yaml:"base_dir"`
	Base    string `yaml:"base"`
}

func Load(flagBase, configPath string) (Config, error) {
	base := flagBase
	if base == "" {
		cfg, err := loadFileConfig(configPath)
		if err != nil {
			return Config{}, err
		}
		base = cfg.BaseDir
	}
	if base == "" {
		path, err := resolveConfigPath(configPath)
		if err != nil {
			return Config{}, err
		}
		return Config{}, errors.New("missing base dir: set --base or base_dir in " + path)
	}

	abs, err := filepath.Abs(base)
	if err != nil {
		return Config{}, err
	}

	return Config{BaseDir: abs}, nil
}

func loadFileConfig(configPath string) (Config, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && configPath == "" {
			return Config{}, nil
		}
		return Config{}, err
	}

	var raw fileConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, err
	}

	base := raw.BaseDir
	if base == "" {
		base = raw.Base
	}
	return Config{BaseDir: base}, nil
}

func resolveConfigPath(configPath string) (string, error) {
	if configPath != "" {
		return filepath.Abs(configPath)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gm", "config.yaml"), nil
}

func StatePath(configPath, name string) (string, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), name), nil
}
