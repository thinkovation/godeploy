# GoDeploy

A simple Go-based deployment tool for deploying Go applications to remote servers via SSH.

## Features

- Build Go applications for Linux/AMD64
- Deploy the binary to a remote server via SSH
- Copy additional files/directories to the remote server
- Run commands on the remote server
- Automatic service restart
- GitHub Actions workflow generation

## Installation

```bash
go install github.com/yourusername/godeploy/cmd/godeploy@latest
```

Or build from source:

```bash
git clone https://github.com/yourusername/godeploy.git
cd godeploy
go build -o godeploy cmd/godeploy/main.go
```

## Usage

### CLI Options

```
Usage of godeploy:
  -config string
        Path to config file (optional)
  -entry string
        Go entry file (default "main.go")
  -file value
        Additional files/folders to copy (format: src[:dst], can be specified multiple times)
  -github
        Generate GitHub Actions workflow file
  -host string
        Remote host address
  -key string
        Path to private key (PEM format)
  -output string
        Name of output binary (default "myapp")
  -path string
        Remote path to copy the binary (default "/home/ubuntu/app")
  -port string
        SSH port (default "22")
  -service string
        Systemd service to restart
  -user string
        SSH username (default "ubuntu")
  -cmd value
        Additional commands to run on remote server (can be specified multiple times)
```

### Configuration File

You can create a configuration file instead of using CLI options. See `deploy.conf.json.example` for an example.

```json
{
  "user": "ubuntu",
  "host": "example.com",
  "port": "22",
  "key": "~/.ssh/id_rsa",
  "path": "/home/ubuntu/app",
  "service": "myapp",
  "output": "myapp",
  "entry": "main.go",
  "commands": [
    "deploy",
    "copyfiles",
    "echo 'Deployment complete!'"
  ],
  "files": [
    {
      "src": "assets",
      "dst": "/home/ubuntu/app/assets",
      "is_dir": true,
      "executable": false
    }
  ]
}
```

### Special Commands

The following commands have special meanings:

- `deploy`: Deploy the binary to the remote server
- `copyfiles`: Copy all specified files/directories to the remote server

### Examples

Build and deploy with a configuration file:

```bash
godeploy -config deploy.conf.json
```

Build and deploy manually:

```bash
godeploy -host example.com -user ubuntu -key ~/.ssh/id_rsa -path /home/ubuntu/app -service myapp -cmd deploy -cmd copyfiles
```

Add files to copy:

```bash
godeploy -host example.com -file assets:/home/ubuntu/app/assets -file config.yaml -cmd deploy -cmd copyfiles
```

Generate GitHub Actions workflow:

```bash
godeploy -config deploy.conf.json -github
```

## GitHub Actions Integration

You can generate a GitHub Actions workflow file by running:

```bash
godeploy -config deploy.conf.json -github
```

This will create:
- `.github/workflows/deploy.yml` - The workflow file
- `GITHUB_SETUP.md` - Instructions for setting up GitHub secrets

## License

MIT