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
func RunCommand(client *ssh.Client, cmd string) error {

	log.Printf("Running: %s", cmd)
	if err := client.RunCommand(cmd); err != nil {
		return fmt.Errorf("command failed: %w", err)
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

	if cfg.Host == "" || cfg.KeyPath == "" {
		return fmt.Errorf("host and key are required for deploy or copyfiles commands")
	}

	// Setup SSH client
	log.Printf("Connecting to %s@%s:%s using key: %s", cfg.User, cfg.Host, cfg.Port, cfg.KeyPath)
	client, err := ssh.NewClient(cfg.User, cfg.Host, cfg.Port, cfg.KeyPath)
	if err != nil {
		log.Printf("Could not establish SSH connection: %v", err)

		return err
	}
	defer client.Close()

	log.Printf("SSH connection established successfully")
	if len(cfg.Commands) > 0 {
		for _, cmd := range cfg.Commands {
			if cmd == "deploy" {
				log.Println("📦 Deploying binary to remote server...")
				if err := DeployBinary(client, cfg, buildDir); err != nil {
					return err
				}
			} else if cmd == "copyfiles" {
				if len(cfg.Files) > 0 {
					log.Println("📂 Copying additional files and directories...")
					if err := DeployFiles(client, cfg); err != nil {
						return err
					}
				} else {
					log.Println("No files to copy, skipping copyfiles command.")
				}

			} else {
				// Keep all non-special commands
				if err := RunCommand(client, cmd); err != nil {
					return err
				}
			}
		}

		log.Println("✅ Remote operations completed successfully.")
	} else {
		// Just report that the build is done
		log.Println("✅ Build completed successfully.")
	}

	return nil
}
