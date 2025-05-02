package config

import (
	"encoding/json"
	"flag"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FileMapping represents a source file/directory and its remote destination
type FileMapping struct {
	Source      string `json:"src"`
	Destination string `json:"dst"`
	IsDir       bool   `json:"is_dir"`
	Executable  bool   `json:"executable"`
}

// Config holds all configuration for the deployment
type Config struct {
	User     string        `json:"user"`
	Host     string        `json:"host"`
	Port     string        `json:"port"`
	KeyPath  string        `json:"key"`
	Path     string        `json:"path"`
	Service  string        `json:"service"`
	Output   string        `json:"output"`
	Entry    string        `json:"entry"`
	Commands []string      `json:"commands"`
	Files    []FileMapping `json:"files"`
}

// LoadConfig loads configuration from a JSON file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// multiStringFlag allows for multiple flag values
type multiStringFlag []string

func (m *multiStringFlag) String() string {
	return strings.Join(*m, ", ")
}

func (m *multiStringFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// fileMappingFlag allows for source:destination pairs
type fileMappingFlag []FileMapping

func (f *fileMappingFlag) String() string {
	var result []string
	for _, mapping := range *f {
		result = append(result, strings.Join([]string{mapping.Source, mapping.Destination}, ":"))
	}
	return strings.Join(result, ", ")
}

func (f *fileMappingFlag) Set(value string) error {
	// Check for executable flag in format: src:dst:x
	var executable bool

	// First check if there's an executable flag
	if strings.HasSuffix(value, ":x") {
		executable = true
		value = value[:len(value)-2]
	}

	// Now handle src:dst part
	parts := strings.SplitN(value, ":", 2)

	mapping := FileMapping{
		Source:     parts[0],
		IsDir:      false, // Will be checked later
		Executable: executable,
	}

	if len(parts) > 1 && parts[1] != "" {
		mapping.Destination = parts[1]
	} else {
		// Default destination is empty and will be filled in later
		mapping.Destination = ""
	}

	*f = append(*f, mapping)
	return nil
}

// NewConfigFromFlags parses command line flags and returns a Config
func NewConfigFromFlags() (*Config, []string, bool, error) {
	var cfg Config
	var commands multiStringFlag
	var fileFlags fileMappingFlag
	
	// Define and parse CLI flags
	configPath := flag.String("config", "", "Path to config file (optional)")
	user := flag.String("user", "ubuntu", "SSH username")
	host := flag.String("host", "", "Remote host address")
	port := flag.String("port", "22", "SSH port")
	keyPath := flag.String("key", "", "Path to private key (PEM format)")
	remotePath := flag.String("path", "/home/ubuntu/app", "Remote path to copy the binary")
	service := flag.String("service", "", "Systemd service to restart")
	output := flag.String("output", "myapp", "Name of output binary")
	entry := flag.String("entry", "main.go", "Go entry file")
	generateGitHubAction := flag.Bool("github", false, "Generate GitHub Actions workflow file")
	
	flag.Var(&commands, "cmd", "Additional commands to run on remote server (can be specified multiple times)")
	flag.Var(&fileFlags, "file", "Additional files/folders to copy (format: src[:dst], can be specified multiple times)")
	flag.Parse()

	// Load config from file if specified
	if *configPath != "" {
		fileCfg, err := LoadConfig(*configPath)
		if err != nil {
			return nil, nil, false, err
		}
		cfg = *fileCfg
	}

	// Override config with CLI flags
	overrideWithFlags(&cfg, *user, *host, *port, *keyPath, *remotePath, *service, *output, *entry)

	// Use config values for commands and files if available
	if len(commands) == 0 && len(cfg.Commands) > 0 {
		commands = cfg.Commands
	}
	if len(fileFlags) == 0 && len(cfg.Files) > 0 {
		fileFlags = cfg.Files
	}

	// Process file mappings
	for i := range fileFlags {
		fileFlags[i] = processFileMapping(fileFlags[i], *remotePath)
	}

	// Update config with processed values
	cfg.Files = fileFlags
	cfg.Commands = commands

	return &cfg, flag.Args(), *generateGitHubAction, nil
}

// overrideWithFlags updates config values with CLI flags when provided
func overrideWithFlags(cfg *Config, user, host, port, keyPath, remotePath, service, output, entry string) {
	// Only override if flag is non-default and config value is not set
	if user != "ubuntu" || cfg.User == "" {
		cfg.User = user
	}
	if host != "" || cfg.Host == "" {
		cfg.Host = host
	}
	if port != "22" || cfg.Port == "" {
		cfg.Port = port
	}
	if keyPath != "" || cfg.KeyPath == "" {
		cfg.KeyPath = keyPath
	}
	if remotePath != "/home/ubuntu/app" || cfg.Path == "" {
		cfg.Path = remotePath
	}
	if service != "" || cfg.Service == "" {
		cfg.Service = service
	}
	if output != "myapp" || cfg.Output == "" {
		cfg.Output = output
	}
	if entry != "main.go" || cfg.Entry == "" {
		cfg.Entry = entry
	}
}

// processFileMapping handles file mapping details like checking if it's a directory
func processFileMapping(mapping FileMapping, remotePath string) FileMapping {
	info, err := os.Stat(mapping.Source)
	if err == nil {
		mapping.IsDir = info.IsDir()
	}

	// If destination is empty, use the base name
	if mapping.Destination == "" {
		mapping.Destination = path.Join(remotePath, filepath.Base(mapping.Source))
	} else if !strings.HasPrefix(mapping.Destination, "/") {
		// If destination is relative path, append to remotePath
		mapping.Destination = path.Join(remotePath, mapping.Destination)
	}

	return mapping
}