# GoDeploy

A simple Go-based deployment tool for Go applications.

## Features

- Build Go applications
- Copy binaries and additional files to remote servers
- Execute remote commands
- Restart services
- Recursive directory copying
- GitHub Actions workflow generation

## Usage

```bash
# Basic usage
godeploy -host your-server.com -key ~/.ssh/id_rsa -service your-app

# With config file
godeploy -config deploy.conf.json

# Copy additional files (simple way)
godeploy -config deploy.conf.json -file config.yaml -file static

# Copy files with custom destinations
godeploy -config deploy.conf.json -file "config.yaml:/etc/myapp/config.yaml" -file "assets:/var/www/assets"

# Copy files with executable permission
godeploy -config deploy.conf.json -file "scripts/setup.sh:/home/ubuntu/myapp/setup.sh:x"

# Run additional commands
godeploy -config deploy.conf.json -cmd "chmod +x /home/ubuntu/myapp" -cmd "echo 'Deployment complete'"

# Generate GitHub Actions workflow file
godeploy -config deploy.conf.json -github
```

## Using a Configuration File

Create a file named `deploy.conf.json` in your project directory. The tool supports Windows, Linux, and macOS-style paths for SSH keys.

> **Note**: A sample configuration file is included as `deploy.conf.json.example`. Copy this file to `deploy.conf.json` and customize it for your environment. The `deploy.conf.json` file is included in `.gitignore` to prevent committing sensitive information.

```json
{
  "user": "gary",
  "host": "139.59.179.21",
  "port": "22",
  "key": "C:/Users/gary/.ssh/id_rsa",
  "path": "/home/gary/myapp",
  "service": "",
  "output": "myapp",
  "entry": "main.go",
  "commands": [
    "chmod +x /home/gary/myapp/myapp",
    "echo 'Running additional setup'"
  ],
  "files": [
    {
      "src": "config.yaml",
      "dst": "/home/gary/myapp/config.yaml",
      "is_dir": false
    },
    {
      "src": "assets",
      "dst": "/home/gary/myapp/assets",
      "is_dir": true
    },
    {
      "src": "README.md",
      "dst": "/home/gary/myapp/docs/DEPLOY.md",
      "is_dir": false
    }
  ]
}
```

Then run:

```bash
godeploy -config deploy.conf.json
```

You can override config file values using command-line flags. For example:

```bash
godeploy -config deploy.conf.json -host staging-server.com
```

## Configuration Options

### Configuration File Fields

| Field    | Description                                     | Required | Default        |
|----------|-------------------------------------------------|----------|----------------|
| user     | SSH username                                    | No       | "ubuntu"       |
| host     | Remote server hostname or IP address            | Yes      | -              |
| port     | SSH port                                        | No       | "22"           |
| key      | Path to SSH private key (Windows/Linux/Mac)     | Yes      | -              |
| path     | Remote directory to deploy to                   | No       | "/home/user/app" |
| service  | Name of systemd service to restart (optional)   | No       | -              |
| output   | Name of compiled binary                         | No       | "myapp"        |
| entry    | Go entry point file                             | No       | "main.go"      |
| commands | List of commands to run on remote server        | No       | []             |
| files    | List of file mapping objects to copy            | No       | []             |

### File Mapping Object

| Field      | Description                                     | Required | Example             |
|------------|-------------------------------------------------|----------|---------------------|
| src        | Local file or directory path                    | Yes      | "config.yaml"       |
| dst        | Remote destination path                         | Yes      | "/etc/app/config.yaml" |
| is_dir     | Whether the source is a directory               | No       | true/false         |
| executable | Whether to set executable permission (chmod +x) | No       | true/false         |

## Command Line Options

- `-config`: Path to config file (optional)
- `-user`: SSH username (default: "ubuntu")
- `-host`: Remote host address
- `-port`: SSH port (default: "22")
- `-key`: Path to private key (PEM format)
- `-path`: Remote path to copy the binary (default: "/home/ubuntu/app")
- `-service`: Systemd service to restart
- `-output`: Name of output binary (default: "myapp")
- `-entry`: Go entry file (default: "main.go")
- `-cmd`: Additional commands to run (can be specified multiple times)
- `-file`: Additional files/folders to copy (format: src[:dst][:x], can be specified multiple times, ':x' makes the file executable)
- `-github`: Generate GitHub Actions workflow file based on the config file

## Troubleshooting

### SSH Connection Issues

If you get an error like `ssh: handshake failed: ssh: unable to authenticate, attempted methods [none publickey], no supported methods remain`, check:

1. The SSH key file path is correct for your OS:
   - Windows: `C:/Users/username/.ssh/id_rsa` 
   - Linux: `/home/username/.ssh/id_rsa`
   - Mac: `/Users/username/.ssh/id_rsa`

2. The SSH key is in the correct format:
   - The tool only supports unencrypted keys
   - If your key is password-protected, create an unencrypted key using:
     ```
     ssh-keygen -m PEM -f ~/.ssh/godeploy_key -N ""
     ```

3. The username is correct for the remote server

4. Your public key is registered on the remote server
   - Check that your key is in `/home/username/.ssh/authorized_keys` on the remote server

## Using GitHub Actions

GoDeploy can automatically generate GitHub Actions workflow files for continuous deployment:

1. Set up your `deploy.conf.json` file with your deployment configuration
2. Run `godeploy -config deploy.conf.json -github`
3. Follow the instructions in the generated `GITHUB_SETUP.md` file
4. Commit the `.github/workflows/deploy.yml` file to your repository

This will create a workflow that automatically builds and deploys your application when you push to the main branch, or when manually triggered.