package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// fileConfig mirrors config.yaml. Leaves are *string so absent keys stay nil and every
// value goes through the same parser as its environment variable. YAML scalars of any
// type (e.g. port: 8460) decode into a string.
type fileConfig struct {
	Server fileServer `yaml:"server"`
	Log    fileLog    `yaml:"log"`
	Paths  filePaths  `yaml:"paths"`
}

type fileServer struct {
	Host *string `yaml:"host"`
	Port *string `yaml:"port"`
}

type fileLog struct {
	Level  *string `yaml:"level"`
	Format *string `yaml:"format"`
}

type filePaths struct {
	Data      *string `yaml:"data"`
	Cache     *string `yaml:"cache"`
	Transcode *string `yaml:"transcode"`
}

// values maps each key path to its value in the file, nil when absent.
func (file fileConfig) values() map[string]*string {
	return map[string]*string{
		keyServerHost:     file.Server.Host,
		keyServerPort:     file.Server.Port,
		keyLogLevel:       file.Log.Level,
		keyLogFormat:      file.Log.Format,
		keyPathsData:      file.Paths.Data,
		keyPathsCache:     file.Paths.Cache,
		keyPathsTranscode: file.Paths.Transcode,
	}
}

// applyFile applies config.yaml from configDir if it exists. It never creates the file.
func applyFile(cfg *Config, configDir string) error {
	file := filepath.Join(configDir, FileName)

	data, err := os.ReadFile(filepath.Clean(file))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read %s: %w", file, err)
	}

	parsed, err := decodeFile(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", file, err)
	}

	values := parsed.values()
	for _, entry := range settings() {
		raw := values[entry.key]
		if raw == nil {
			continue
		}

		if err := entry.apply(cfg, *raw, configDir); err != nil {
			return fmt.Errorf("%s: key %s: %w", file, entry.key, err)
		}
	}

	return nil
}

// decodeFile parses the YAML strictly: unknown keys are errors that name the key.
func decodeFile(data []byte) (fileConfig, error) {
	var parsed fileConfig

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	// io.EOF means the file has no document (empty or only comments): nothing to apply.
	// Other errors already start with "yaml:"; the caller adds the file path.
	if err := decoder.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return fileConfig{}, err
	}

	return parsed, nil
}

// applyEnvironment applies every set environment variable. A variable set to an empty
// value counts as set, so empty paths are reported instead of silently ignored.
func applyEnvironment(cfg *Config, env paths.Environment, configDir string) error {
	for _, entry := range settings() {
		name := envName(entry.key)

		raw, ok := env.Lookup(name)
		if !ok {
			continue
		}

		if err := entry.apply(cfg, raw, configDir); err != nil {
			return fmt.Errorf("environment variable %s: %w", name, err)
		}
	}

	return nil
}
