package ssh

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client wraps an SSH client connection
type Client struct {
	client *ssh.Client
}

// NewClient creates a new SSH client connection
func NewClient(user, host, port, keyPath string) (*Client, error) {
	signer, err := loadPrivateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	address := fmt.Sprintf("%s:%s", host, port)
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %w\nPlease verify your SSH key, username, and server address", err)
	}

	return &Client{client: client}, nil
}

// Close closes the SSH connection
func (c *Client) Close() error {
	return c.client.Close()
}

// CopyFile copies a local file to a remote path
func (c *Client) CopyFile(localPath, remotePath string, executable bool) error {
	session, err := c.client.NewSession()
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
		return fmt.Errorf("remote SCP failed: %w", err)
	}
	return nil
}

// CopyDirectory copies a local directory to a remote path recursively
func (c *Client) CopyDirectory(localPath, remotePath string, executableFiles map[string]bool) error {
	// First ensure the remote directory exists
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remotePath)
	if err := c.RunCommand(mkdirCmd); err != nil {
		return fmt.Errorf("failed to create remote directory: %w", err)
	}

	// Get list of files in the directory
	entries, err := os.ReadDir(localPath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(localPath, entry.Name())
		dstPath := path.Join(remotePath, entry.Name())

		if entry.IsDir() {
			// Recursively copy directory
			if err := c.CopyDirectory(srcPath, dstPath, executableFiles); err != nil {
				return err
			}
		} else {
			// Check if file should be executable (using the full path)
			executable := executableFiles[srcPath]

			// Copy file
			if err := c.CopyFile(srcPath, dstPath, executable); err != nil {
				return err
			}
		}
	}

	return nil
}

// RunCommand executes a command on the remote server
func (c *Client) RunCommand(command string) error {
	session, err := c.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var out bytes.Buffer
	var stderr bytes.Buffer
	session.Stdout = &out
	session.Stderr = &stderr

	if err := session.Run(command); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	fmt.Print(out.String())
	return nil
}

// RunLocalCommand runs a command on the local machine
func RunLocalCommand(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// loadPrivateKey loads an SSH private key from a file
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
				return nil, fmt.Errorf("failed to read SSH key: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read SSH key: %w", err)
		}
	}

	// Try to parse the key
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		// Check if this might be an encrypted key
		if strings.Contains(err.Error(), "cannot decode encrypted private keys") {
			return nil, fmt.Errorf("encrypted SSH key detected: %w. Use ssh-keygen to create an unencrypted key", err)
		}
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}

	return signer, nil
}