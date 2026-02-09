# FSAK - Fast Secure Awesome Kokh

[Persian (فارسی)](README_fa.md)

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
  "addresses": [
    "1.1.1.1", 
    "2.2.2.0/24", 
    "3.3.3.3-4.4.4.4"
  ],                         // Server addresses (supports IP, CIDR, and Ranges)
  "host": "your-cdn-host.com", // Host header / SNI
  "tls": false,              // Enable TLS
  "sni": "your-cdn-host.com", // Server Name Indication (if TLS is true)
  "port": 80,                // Server listening port
  "proxy_port": 1080,        // Local SOCKS5 port (for client)
  "secret": "my-secret-key"  // Shared secret for authentication
}
```

> [!IMPORTANT]
> **CDN & Cloudflare Configuration:**
> - The connection between the **CDN** and your **Server** must be over **HTTP** (not HTTPS).
> - If you are using **Cloudflare**, you must set the SSL/TLS encryption mode to **Flexible**.

## Usage

### Running the Server

1.  Create a `config.json` (or `server_config.json`) with the desired port and secret.
2.  Run the server:

```bash
./bin/fsak-server -config config.json
```

![Server Screenshot](resource/img/server.png)

### Running the Client

1.  Create a `config.json` with the server's address, the shared secret, and your desired local SOCKS5 port.
2.  Run the client:

```bash
./bin/fsak-client -config config.json
```

![Client Screenshot](resource/img/client.png)

3.  Configure your browser or application to use the SOCKS5 proxy at `127.0.0.1:1080` (or whatever `proxy_port` you configured).

## License

MIT


## زنده باد ایران - به امید آزادی 