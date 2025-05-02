package deploy

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"godeploy/internal/config"
	"godeploy/internal/ssh"
)

// Build compiles the Go application with the provided configuration
func Build(cfg *config.Config, buildDir string) error {
	// Ensure build directory exists
	if _, err := os.Stat(buildDir); os.IsNotExist(err) {
		log.Printf("Creating build directory: %s", buildDir)
		if err := os.MkdirAll(buildDir, 0755); err != nil {
			return fmt.Errorf("failed to create build directory: %w", err)
		}
	}

	outputPath := filepath.Join(buildDir, cfg.Output)
	buildCmd := exec.Command("go", "build", "-o", outputPath, cfg.Entry)
	buildCmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")

	return buildCmd.Run()
}

// DeployBinary copies the binary to the remote server
func DeployBinary(client *ssh.Client, cfg *config.Config, buildDir string) error {
	outputPath := filepath.Join(buildDir, cfg.Output)
	return client.CopyFile(outputPath, filepath.Join(cfg.Path, cfg.Output), true)
}

// DeployFiles copies additional files to the remote server
func DeployFiles(client *ssh.Client, cfg *config.Config) error {
	// Create a map of files that should be executable
	executableFiles := make(map[string]bool)
	for _, mapping := range cfg.Files {
		if mapping.Executable {
			executableFiles[mapping.Source] = true
		}
	}

	// Copy each file/directory
	for _, mapping := range cfg.Files {
		log.Printf("Copying %s to %s", mapping.Source, mapping.Destination)

		if mapping.IsDir {
			if err := client.CopyDirectory(mapping.Source, mapping.Destination, executableFiles); err != nil {
				return fmt.Errorf("directory copy failed for %s: %w", mapping.Source, err)
			}
		} else {
			if err := client.CopyFile(mapping.Source, mapping.Destination, mapping.Executable); err != nil {
				return fmt.Errorf("file copy failed for %s: %w", mapping.Source, err)
			}
		}
	}

	return nil
}

// RunCommands executes commands on the remote server
func RunCommands(client *ssh.Client, commands []string) error {
	for _, cmd := range commands {
		log.Printf("Running: %s", cmd)
		if err := client.RunCommand(cmd); err != nil {
			return fmt.Errorf("command failed: %w", err)
		}
	}
	return nil
}

// RunLocalCommands executes commands locally
func RunLocalCommands(commands []string) error {
	for _, cmd := range commands {
		log.Printf("Running local command: %s", cmd)
		if err := ssh.RunLocalCommand(cmd); err != nil {
			return fmt.Errorf("local command failed: %w", err)
		}
	}
	return nil
}

// Process handles the deployment process
func Process(cfg *config.Config) error {
	buildDir := "build"
	
	// Step 1: Always build first
	log.Println("🔨 Building Go binary...")
	if err := Build(cfg, buildDir); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Process commands to identify special operations
	var needDeploy, needCopyFiles bool
	var filteredCommands []string
	
	for _, cmd := range cfg.Commands {
		if cmd == "deploy" {
			needDeploy = true
		} else if cmd == "copyfiles" {
			needCopyFiles = true
		} else {
			// Keep all non-special commands
			filteredCommands = append(filteredCommands, cmd)
		}
	}

	// If we need to do remote operations, establish SSH connection
	if needDeploy || needCopyFiles {
		if cfg.Host == "" || cfg.KeyPath == "" {
			return fmt.Errorf("host and key are required for deploy or copyfiles commands")
		}

		// Setup SSH client
		log.Printf("Connecting to %s@%s:%s using key: %s", cfg.User, cfg.Host, cfg.Port, cfg.KeyPath)
		client, err := ssh.NewClient(cfg.User, cfg.Host, cfg.Port, cfg.KeyPath)
		if err != nil {
			return err
		}
		defer client.Close()

		log.Printf("SSH connection established successfully")

		// Deploy binary if 'deploy' command is present
		if needDeploy {
			log.Println("📦 Deploying binary to remote server...")
			if err := DeployBinary(client, cfg, buildDir); err != nil {
				return err
			}

			// Add service restart command if service is specified
			if cfg.Service != "" {
				serviceRestartCmd := fmt.Sprintf("sudo systemctl restart %s", cfg.Service)
				filteredCommands = append(filteredCommands, serviceRestartCmd)
			}
		}

		// Copy additional files if 'copyfiles' command is present
		if needCopyFiles && len(cfg.Files) > 0 {
			log.Println("📂 Copying additional files and directories...")
			if err := DeployFiles(client, cfg); err != nil {
				return err
			}
		}

		// Run all remaining commands
		if len(filteredCommands) > 0 {
			log.Println("🔧 Running remote commands...")
			if err := RunCommands(client, filteredCommands); err != nil {
				return err
			}
		}
		
		log.Println("✅ Remote operations completed successfully.")
	} else if len(filteredCommands) > 0 {
		// If no remote operations but we have commands, run them locally
		log.Println("🔧 Running local commands...")
		if err := RunLocalCommands(filteredCommands); err != nil {
			return err
		}
		log.Println("✅ Local commands completed successfully.")
	} else {
		// Just report that the build is done
		log.Println("✅ Build completed successfully.")
	}
	
	return nil
}