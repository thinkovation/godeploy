package github

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"

	"godeploy/internal/config"
)

// GenerateWorkflow creates GitHub Actions workflow file and instructions
func GenerateWorkflow(configPath string) error {
	// Load the config file
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create .github/workflows directory if it doesn't exist
	workflowDir := ".github/workflows"
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		return fmt.Errorf("failed to create workflows directory: %w", err)
	}

	// Generate the workflow YAML file
	workflowContent := generateWorkflowYAML(cfg)
	workflowPath := filepath.Join(workflowDir, "deploy.yml")
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		return fmt.Errorf("failed to write workflow file: %w", err)
	}
	log.Printf("✅ Created GitHub Actions workflow: %s", workflowPath)

	// Generate instructions for setting up secrets
	instructions := generateSecretInstructions(cfg)
	instructionsPath := "GITHUB_SETUP.md"
	if err := os.WriteFile(instructionsPath, []byte(instructions), 0644); err != nil {
		return fmt.Errorf("failed to write instructions: %w", err)
	}
	log.Printf("✅ Created GitHub setup instructions: %s", instructionsPath)

	return nil
}

// generateWorkflowYAML creates a GitHub Actions workflow file for deployment
func generateWorkflowYAML(cfg *config.Config) string {
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
func generateSecretInstructions(cfg *config.Config) string {
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