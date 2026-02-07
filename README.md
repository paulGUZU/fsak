# FSAK - Fast Secure Awesome Kokh

FSAK is a high-performance, secure SOCKS5 proxy server and client written in Go. It allows you to tunnel traffic securely between a client and a server, bypassing restrictions and ensuring privacy.

## Features

- **High Performance**: Built with Go for concurrency and speed.
- **SOCKS5 Support**: Standard SOCKS5 protocol support.
- **Secure Transport**: potentially supports TLS (configuration dependent).
- **Easy Configuration**: JSON-based configuration.
- **Cross-Platform**: Runs on Linux, Windows, and macOS.

## Installation

### Prerequisites

- Go 1.25+ (for building from source)

### Building from Source

To build the client and server binaries:

```bash
# Clone the repository
git clone https://github.com/paulGUZU/fsak.git
cd fsak

# Build Client
go build -o bin/fsak-client ./cmd/client

# Build Server
go build -o bin/fsak-server ./cmd/server
```

## Configuration

Both client and server use a `config.json` file.

### Example `config.json`

```json
{
  "addresses": ["127.0.0.1"],  // Server addresses for the client to connect to
  "host": "localhost",       // Host header / SNI
  "tls": false,              // Enable TLS
  "sni": "localhost",        // Server Name Indication (if TLS is true)
  "port": 8080,              // Server listening port (for server)
  "proxy_port": 1080,        // Local SOCKS5 port (for client)
  "secret": "my-secret-key"  // Shared secret for authentication
}
```

## Usage

### Running the Server

1.  Create a `config.json` (or `server_config.json`) with the desired port and secret.
2.  Run the server:

```bash
./bin/fsak-server -config config.json
```

### Running the Client

1.  Create a `config.json` with the server's address, the shared secret, and your desired local SOCKS5 port.
2.  Run the client:

```bash
./bin/fsak-client -config config.json
```

3.  Configure your browser or application to use the SOCKS5 proxy at `127.0.0.1:1080` (or whatever `proxy_port` you configured).

## License

MIT
