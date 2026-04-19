package exampleconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/ego-component/eamqp"
	"github.com/gotomicro/ego/core/econf"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultComponentKey matches examples/config/local.toml.
	DefaultComponentKey = "amqp.default"
	// ConfigEnvName allows running examples from any working directory.
	ConfigEnvName = "EAMQP_EXAMPLE_CONFIG"
)

// LoadClient loads an Ego config file and builds an eamqp client from componentKey.
func LoadClient(componentKey string) (*eamqp.Client, error) {
	if componentKey == "" {
		componentKey = DefaultComponentKey
	}

	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}

	path, err := ResolveConfigPath(os.Args, os.Getenv, wd)
	if err != nil {
		return nil, err
	}
	if err := LoadConfigFile(path); err != nil {
		return nil, err
	}

	return eamqp.Load(componentKey).BuildE(eamqp.WithOnFail("error"))
}

// Args returns command-line arguments with example-only config flags removed.
func Args() []string {
	return ArgsWithoutConfig(os.Args)
}

// ArgsWithoutConfig removes --config/-config flags and the executable name.
func ArgsWithoutConfig(args []string) []string {
	if len(args) == 0 {
		return nil
	}

	result := make([]string, 0, len(args)-1)
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--config=") || strings.HasPrefix(arg, "-config=") {
			continue
		}
		result = append(result, arg)
	}
	return result
}

// ResolveConfigPath resolves the config file path for standalone examples.
func ResolveConfigPath(args []string, getenv func(string) string, workingDir string) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	if path := configPathFromArgs(args); path != "" {
		return path, nil
	}
	if path := getenv(ConfigEnvName); path != "" {
		return path, nil
	}

	for _, path := range defaultConfigCandidates(workingDir) {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("eamqp example config not found: pass --config=/path/to/local.toml or set %s", ConfigEnvName)
}

// LoadConfigFile loads TOML, YAML, or JSON into Ego's global configuration.
func LoadConfigFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open config %s: %w", path, err)
	}
	defer file.Close()

	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		return econf.LoadFromReader(file, toml.Unmarshal)
	case ".yaml", ".yml":
		return econf.LoadFromReader(file, yaml.Unmarshal)
	case ".json":
		return econf.LoadFromReader(file, json.Unmarshal)
	default:
		return fmt.Errorf("unsupported config format %s: use .toml, .yaml, .yml, or .json", filepath.Ext(path))
	}
}

func configPathFromArgs(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-config" {
			if i+1 < len(args) {
				return args[i+1]
			}
			return ""
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
		if strings.HasPrefix(arg, "-config=") {
			return strings.TrimPrefix(arg, "-config=")
		}
	}
	return ""
}

func defaultConfigCandidates(workingDir string) []string {
	return []string{
		filepath.Join(workingDir, "examples", "config", "local.toml"),
		filepath.Join(workingDir, "..", "config", "local.toml"),
		filepath.Join(workingDir, "config", "local.toml"),
	}
}
