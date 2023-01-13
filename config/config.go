package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

var DefaultLocalPath = ".devx-config"
var DefaultEC2Path = "/etc/config/tags.json" // set by Amigo 'cdk-base' role

type Config struct {
	Stack, Stage, App string
}

func (c *Config) Unmarshal(data []byte) error {
	return json.Unmarshal(data, c)
}

func Merge(configs ...Config) Config {
	var out Config

	for _, config := range configs {
		if config.App != "" {
			out.App = config.App
		}
		if config.Stack != "" {
			out.Stack = config.Stack
		}
		if config.Stage != "" {
			out.Stage = config.Stage
		}
	}

	return out
}

func DefaultFiles() []io.ReadCloser {
	paths := []string{DefaultLocalPath, DefaultEC2Path}
	files := []io.ReadCloser{}

	for _, path := range paths {
		file, err := os.Open(path)
		if err == nil {
			files = append(files, file)
		}
	}

	return files
}

// Reads any file configs and merges with passed arg values. When both present,
// the arg value is preferred. Only the first file that contains config data is
// used.
func Read(argConfig Config, files ...io.ReadCloser) (Config, error) {
	fileConfig := Config{}

	for _, f := range files {
		defer f.Close()
		data, err := io.ReadAll(f)
		if err == nil {
			err = fileConfig.Unmarshal(data)
			if err != nil {
				return fileConfig, err
			}

			break
		}
	}

	merged := Merge(fileConfig, argConfig)

	if merged.App == "" || merged.Stack == "" || merged.Stage == "" {
		return merged, fmt.Errorf("mandatory flag missing or empty (got app='%s', stack='%s', stage='%s')", merged.App, merged.Stack, merged.Stage)
	}

	return merged, nil
}

func Write(config Config) error {
	out, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("unable to marshal JSON: %w", err)
	}

	err = os.WriteFile(DefaultLocalPath, out, 0644)
	if err != nil {
		return fmt.Errorf("unable to write config file: %w", err)
	}

	return nil
}
