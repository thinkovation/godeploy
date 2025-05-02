package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type FileMapping struct {
	Source      string `json:"src"`
	Destination string `json:"dst"`
	IsDir       bool   `json:"is_dir"`
	Executable  bool   `json:"executable"`
}

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

func loadConfig(path string) (*Config, error) {
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
		result = append(result, fmt.Sprintf("%s:%s", mapping.Source, mapping.Destination))
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
		// Default destination is just the basename in the remote path
		mapping.Destination = ""
	}

	*f = append(*f, mapping)
	return nil
}

func main() {
	// CLI flags
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
	var commands multiStringFlag
	flag.Var(&commands, "cmd", "Additional commands to run on remote server (can be specified multiple times)")
	var fileFlags fileMappingFlag
	flag.Var(&fileFlags, "file", "Additional files/folders to copy (format: src[:dst], can be specified multiple times)")
	flag.Parse()

	var cfg Config
	if *configPath != "" {
		fileCfg, err := loadConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		cfg = *fileCfg
	}
	// Use config values if CLI flags are default
	if *user == "ubuntu" && cfg.User != "" {
		*user = cfg.User
	}
	if *host == "" && cfg.Host != "" {
		*host = cfg.Host
	}
	if *port == "22" && cfg.Port != "" {
		*port = cfg.Port
	}
	if *keyPath == "" && cfg.KeyPath != "" {
		*keyPath = cfg.KeyPath
	}
	if *remotePath == "/home/ubuntu/app" && cfg.Path != "" {
		*remotePath = cfg.Path
	}
	if *service == "" && cfg.Service != "" {
		*service = cfg.Service
	}
	if *output == "myapp" && cfg.Output != "" {
		*output = cfg.Output
	}
	if *entry == "main.go" && cfg.Entry != "" {
		*entry = cfg.Entry
	}

	// Use config values for commands and files if available
	if len(commands) == 0 && len(cfg.Commands) > 0 {
		commands = cfg.Commands
	}
	if len(fileFlags) == 0 && len(cfg.Files) > 0 {
		// Convert config files to file flags
		fileFlags = cfg.Files
	}

	// Check which file mappings are directories
	for i := range fileFlags {
		info, err := os.Stat(fileFlags[i].Source)
		if err != nil {
			log.Printf("Warning: cannot access file %s: %v", fileFlags[i].Source, err)
			continue
		}
		fileFlags[i].IsDir = info.IsDir()

		// If destination is empty, use the base name
		if fileFlags[i].Destination == "" {
			fileFlags[i].Destination = path.Join(*remotePath, filepath.Base(fileFlags[i].Source))
		} else if !strings.HasPrefix(fileFlags[i].Destination, "/") {
			// If destination is relative path, append to remotePath
			fileFlags[i].Destination = path.Join(*remotePath, fileFlags[i].Destination)
		}
	}

	// Check if we should generate GitHub Actions workflow file
	if *generateGitHubAction {
		if *configPath == "" {
			log.Fatal("Config file is required for GitHub Actions generation")
		}
		if err := generateGitHubWorkflow(*configPath); err != nil {
			log.Fatalf("Failed to generate GitHub Actions workflow: %v", err)
		}
		return
	}

	if *host == "" || *keyPath == "" {
		log.Fatal("host and key are required")
	}

	// Step 1: Build
	log.Println("🔨 Building Go binary...")
	buildCmd := exec.Command("go", "build", "-o", *output, *entry)
	buildCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	if err := buildCmd.Run(); err != nil {
		log.Fatalf("Build failed: %v", err)
	}

	// Step 2: Setup SSH client
	log.Printf("Connecting to %s@%s:%s using key: %s", *user, *host, *port, *keyPath)
	signer, err := loadPrivateKey(*keyPath)
	if err != nil {
		log.Fatalf("Failed to load private key: %v", err)
	}

	config := &ssh.ClientConfig{
		User:            *user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	address := fmt.Sprintf("%s:%s", *host, *port)
	log.Printf("Dialing SSH connection to %s", address)
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		log.Fatalf("SSH connection failed: %v\nPlease verify your SSH key, username, and server address", err)
	}
	defer client.Close()

	log.Printf("SSH connection established successfully")

	// Step 3: Copy binary via SCP (always executable)
	log.Println("📦 Copying binary to remote server...")
	if err := scpFile(client, *output, path.Join(*remotePath, *output), true); err != nil {
		log.Fatalf("SCP failed: %v", err)
	}

	// Step 3b: Copy additional files if specified
	if len(fileFlags) > 0 {
		log.Println("📂 Copying additional files and directories...")

		// Create a map of files that should be executable
		executableFiles := make(map[string]bool)
		for _, mapping := range fileFlags {
			if mapping.Executable {
				executableFiles[mapping.Source] = true
			}
		}

		for _, mapping := range fileFlags {
			log.Printf("  Copying %s to %s", mapping.Source, mapping.Destination)

			if mapping.IsDir {
				if err := scpDir(client, mapping.Source, mapping.Destination, executableFiles); err != nil {
					log.Fatalf("Directory copy failed for %s: %v", mapping.Source, err)
				}
			} else {
				if err := scpFile(client, mapping.Source, mapping.Destination, mapping.Executable); err != nil {
					log.Fatalf("File copy failed for %s: %v", mapping.Source, err)
				}
			}
		}
	}

	// Step 4: Run commands (including service restart if specified)
	// Add service restart to commands list if service is specified
	if *service != "" {
		serviceRestartCmd := fmt.Sprintf("sudo systemctl restart %s", *service)
		commands = append(commands, serviceRestartCmd)
	}

	if len(commands) > 0 {
		log.Println("🔧 Running commands...")
		for _, cmd := range commands {
			log.Printf("  Running: %s", cmd)
			if err := runRemoteCommand(client, cmd); err != nil {
				log.Fatalf("Command failed: %v", err)
			}
		}
	}

	log.Println("✅ Deployment successful.")
}

func loadPrivateKey(keyPath string) (ssh.Signer, error) {
	// Normalize path for different OS formats
	keyPath = filepath.ToSlash(keyPath)

	key, err := os.ReadFile(keyPath)
	if err != nil {
		// Try to use Windows path as a fallback
		if runtime.GOOS == "windows" {
			keyPath = strings.Replace(keyPath, "/", "\\", -1)
			key, err = os.ReadFile(keyPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read SSH key: %v", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read SSH key: %v", err)
		}
	}

	// Try to parse the key
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		// Check if this might be an encrypted key
		if strings.Contains(err.Error(), "cannot decode encrypted private keys") {
			return nil, fmt.Errorf("encrypted SSH key detected: %v. Use ssh-keygen to create an unencrypted key", err)
		}
		return nil, fmt.Errorf("failed to parse SSH key: %v", err)
	}

	return signer, nil
}

func scpFile(client *ssh.Client, localPath, remotePath string, executable bool) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	srcFile, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	// Set file mode based on executable flag
	var mode int64
	if executable {
		mode = 0755 // rwxr-xr-x (executable)
	} else {
		mode = 0644 // rw-r--r-- (regular file)
	}

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C%04o %d %s\n", mode, info.Size(), path.Base(remotePath))
		io.Copy(w, srcFile)
		fmt.Fprint(w, "\x00")
	}()

	cmd := fmt.Sprintf("scp -t %s", path.Dir(remotePath))
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("remote SCP failed: %v", err)
	}
	return nil
}

func scpDir(client *ssh.Client, localPath, remotePath string, executableFiles map[string]bool) error {
	// First ensure the remote directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remotePath)
	if err := runRemoteCommand(client, mkdirCmd); err != nil {
		return fmt.Errorf("failed to create remote directory: %v", err)
	}

	// Get list of files in the directory
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(localPath, entry.Name())
		dstPath := path.Join(remotePath, entry.Name())

		if entry.IsDir() {
			// Recursively copy directory
			if err := scpDir(client, srcPath, dstPath, executableFiles); err != nil {
				return err
			}
		} else {
			// Check if file should be executable (using the full path)
			executable := executableFiles[srcPath]

			// Copy file
			if err := scpFile(client, srcPath, dstPath, executable); err != nil {
				return err
			}
		}
	}

	return nil
}

func runRemoteCommand(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var out bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &out
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		return fmt.Errorf("%s: %s", err, stderr.String())
	}
	fmt.Print(out.String())
	return nil
}

// generateGitHubWorkflow creates GitHub Actions workflow file and instructions
func generateGitHubWorkflow(configPath string) error {
	// Load the config file
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Create .github/workflows directory if it doesn't exist
	workflowDir := ".github/workflows"
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %v", err)
	}

	// Generate the workflow YAML file
	workflowContent := generateWorkflowYAML(cfg)
	workflowPath := filepath.Join(workflowDir, "deploy.yml")
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		return fmt.Errorf("failed to write workflow file: %v", err)
	}
	log.Printf("✅ Created GitHub Actions workflow: %s", workflowPath)

	// Generate instructions for setting up secrets
	instructions := generateSecretInstructions(cfg)
	instructionsPath := "GITHUB_SETUP.md"
	if err := os.WriteFile(instructionsPath, []byte(instructions), 0644); err != nil {
		return fmt.Errorf("failed to write instructions: %v", err)
	}
	log.Printf("✅ Created GitHub setup instructions: %s", instructionsPath)

	return nil
}

// generateWorkflowYAML creates a GitHub Actions workflow file for deployment
func generateWorkflowYAML(cfg *Config) string {
	// Get the filename for deployment
	deployFilename := cfg.Output
	if deployFilename == "" {
		deployFilename = "myapp"
	}

	// Extract the repository name from the current directory
	repoName := filepath.Base(cfg.Output)
	if repoName == "" || repoName == "." {
		repoName = "app"
	}

	// Construct list of files to copy
	filesCopy := ""
	for _, file := range cfg.Files {
		dst := file.Destination
		if dst == "" {
			dst = path.Join(cfg.Path, filepath.Base(file.Source))
		}
		
		if file.IsDir {
			filesCopy += fmt.Sprintf("          # Copy directory: %s to %s\n", file.Source, dst)
			filesCopy += fmt.Sprintf("          - run: scp -r -i key.pem -P ${{ secrets.SSH_PORT }} %s ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }}:%s\n", file.Source, dst)
		} else {
			filesCopy += fmt.Sprintf("          # Copy file: %s to %s\n", file.Source, dst)
			filesCopy += fmt.Sprintf("          - run: scp -i key.pem -P ${{ secrets.SSH_PORT }} %s ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }}:%s\n", file.Source, dst)
		}
	}

	// Construct remote commands
	commands := ""
	for i, cmd := range cfg.Commands {
		commands += fmt.Sprintf("          # Command %d\n", i+1)
		commands += fmt.Sprintf("          - run: ssh -i key.pem -p ${{ secrets.SSH_PORT }} ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }} '%s'\n", cmd)
	}

	// Add service restart if specified
	if cfg.Service != "" {
		commands += fmt.Sprintf("          # Restart service\n")
		commands += fmt.Sprintf("          - run: ssh -i key.pem -p ${{ secrets.SSH_PORT }} ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }} 'sudo systemctl restart %s'\n", cfg.Service)
	}

	// Generate the workflow file
	workflowYAML := fmt.Sprintf(`name: Deploy %s

on:
  push:
    branches: [ main ]
  workflow_dispatch:  # Allows manual triggering

jobs:
  deploy:
    runs-on: ubuntu-latest
    
    steps:
      - name: Checkout code
        uses: actions/checkout@v3
        
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.20'
          
      - name: Build
        run: |
          GOOS=linux GOARCH=amd64 go build -o %s %s
          
      - name: Setup SSH key
        run: |
          echo "${{ secrets.SSH_PRIVATE_KEY }}" > key.pem
          chmod 600 key.pem
          
      - name: Deploy to server
        run: |
          # Create remote directory if it doesn't exist
          ssh -i key.pem -p ${{ secrets.SSH_PORT }} -o StrictHostKeyChecking=no ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }} "mkdir -p %s"
          
          # Copy app binary
          scp -i key.pem -P ${{ secrets.SSH_PORT }} %s ${{ secrets.SSH_USER }}@${{ secrets.SSH_HOST }}:%s/%s
          chmod +x %s/%s
%s
      - name: Execute remote commands
        run: |
%s
          
      - name: Cleanup
        run: rm -f key.pem
`, 
		repoName, 
		deployFilename, 
		cfg.Entry,
		cfg.Path,
		deployFilename, 
		cfg.Path, 
		deployFilename,
		cfg.Path,
		deployFilename,
		filesCopy,
		commands)

	return workflowYAML
}

// generateSecretInstructions creates instructions for setting up GitHub secrets
func generateSecretInstructions(cfg *Config) string {
	instructions := `# GitHub Actions Setup for Deployment

This project includes a GitHub Actions workflow for automatic deployment. To set up this workflow, you need to add the following secrets to your GitHub repository.

## Required Secrets

| Secret Name        | Description                               | Example Value        |
|--------------------|-------------------------------------------|----------------------|
| SSH_HOST           | The hostname or IP of your server         | example.com          |
| SSH_PORT           | The SSH port for your server              | 22                   |
| SSH_USER           | The SSH username for deployment           | deployer             |
| SSH_PRIVATE_KEY    | The SSH private key (the entire key file) | -----BEGIN...        |

## How to Add Secrets

1. Go to your GitHub repository
2. Click on "Settings" tab
3. In the left sidebar, click on "Secrets and variables" → "Actions"
4. Click on "New repository secret"
5. Add each of the secrets mentioned above

## SSH Key Setup

For security reasons, it's recommended to create a dedicated deployment key:

` + "```bash" + `
# Generate a new SSH key for deployment
ssh-keygen -t ed25519 -f ~/.ssh/github_deploy -C "github-actions-deploy"

# Display the public key to add to your server's authorized_keys file
cat ~/.ssh/github_deploy.pub

# Display the private key to add as a GitHub secret
cat ~/.ssh/github_deploy
` + "```" + `

Add the public key to your server's authorized_keys file:

` + "```bash" + `
echo "$(cat ~/.ssh/github_deploy.pub)" >> ~/.ssh/authorized_keys
` + "```" + `

Copy the entire private key (including BEGIN and END lines) and add it as the SSH_PRIVATE_KEY secret in GitHub.

## Workflow Details

The workflow will:

1. Build the application for Linux
2. Copy the binary to your server
3. Copy any additional files specified in your configuration
4. Run all specified commands on the server
`

	// Add specific configuration details to the instructions
	instructions += fmt.Sprintf(`
## Current Configuration

| Setting            | Value                                     |
|--------------------|-------------------------------------------|
| Host               | %s                                        |
| Port               | %s                                        |
| User               | %s                                        |
| Remote Path        | %s                                        |
| Binary Name        | %s                                        |
| Service            | %s                                        |

When setting up your secrets, use these values or update them as needed.
`, 
		cfg.Host, 
		cfg.Port, 
		cfg.User, 
		cfg.Path, 
		cfg.Output, 
		cfg.Service)

	return instructions
}
