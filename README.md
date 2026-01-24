# grpc-mcp

A dynamic gRPC to MCP (Model Context Protocol) bridge that exposes gRPC services as MCP tools using server reflection. No proto files required - services are discovered at runtime.

## Features

- **Dynamic Discovery**: Automatically discovers gRPC services using server reflection
- **Zero Configuration**: No proto files needed - schemas are generated from reflection data
- **Multiple Endpoints**: Connect to multiple gRPC servers simultaneously
- **All Streaming Types**: Supports unary, server streaming, client streaming, and bidirectional streaming
- **Hot Reload**: Watch config file for changes and update tools dynamically
- **Multiple Auth Methods**: Bearer tokens, API keys, and mutual TLS (mTLS)
- **Health Monitoring**: Automatic health checks with reconnection

## Installation

```bash
# Clone the repository
git clone https://github.com/grpc-mcp/grpc-mcp.git
cd grpc-mcp

# Build
go build -o grpc-mcp-server ./cmd/grpc-mcp-server

# Or install directly
go install github.com/grpc-mcp/grpc-mcp/cmd/grpc-mcp-server@latest
```

## Quick Start

1. Create a configuration file `grpc-mcp.yaml`:

```yaml
server:
  name: "grpc-mcp"
  version: "1.0.0"
  transport: "stdio"

endpoints:
  - name: "local-api"
    address: "localhost:50051"
    auth:
      type: "none"
    exclude:
      - "grpc.reflection.*"
      - "grpc.health.*"
```

2. Run the server:

```bash
./grpc-mcp-server --config ./grpc-mcp.yaml
```

3. Configure in Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "grpc": {
      "command": "/path/to/grpc-mcp-server",
      "args": ["--config", "/path/to/grpc-mcp.yaml"]
    }
  }
}
```

## Configuration

### Server Configuration

```yaml
server:
  name: "grpc-mcp"           # Server name (shown to MCP clients)
  version: "1.0.0"           # Server version
  transport: "stdio"         # Transport: "stdio" or "sse"
  address: ":8080"           # Address for SSE transport
```

### Endpoint Configuration

```yaml
endpoints:
  # Plaintext (development)
  - name: "local-api"
    address: "localhost:50051"
    auth:
      type: "none"
    tls:
      enabled: false
    exclude:
      - "grpc.reflection.*"
      - "grpc.health.*"
    healthCheck:
      enabled: true
      interval: 30s

  # Bearer token authentication
  - name: "auth-api"
    address: "api.example.com:443"
    auth:
      type: "bearer"
      bearerToken: "${AUTH_TOKEN}"  # Environment variable expansion
    tls:
      enabled: true

  # API key authentication
  - name: "external-api"
    address: "external.example.com:443"
    auth:
      type: "api-key"
      apiKey:
        header: "x-api-key"
        value: "${API_KEY}"
    tls:
      enabled: true

  # Mutual TLS (mTLS)
  - name: "secure-internal"
    address: "secure.internal:443"
    auth:
      type: "mtls"
      mtls:
        certFile: "/certs/client.pem"
        keyFile: "/certs/client-key.pem"
        caFile: "/certs/ca.pem"
```

### Environment Variables

Configuration values support environment variable expansion:

- `${VAR}` - Expands to the value of VAR (empty if unset)
- `${VAR:-default}` - Expands to VAR or "default" if unset

## Usage

### Command Line Options

```bash
grpc-mcp-server [flags]

Flags:
  -c, --config string      Path to configuration file (default "grpc-mcp.yaml")
  -t, --transport string   Transport type (stdio or sse), overrides config
  -a, --address string     Address for SSE transport, overrides config
  -w, --watch              Watch config file for changes and hot-reload
  -h, --help               Help for grpc-mcp-server
  -v, --version            Version information
```

### Examples

```bash
# stdio transport (Claude Desktop, Claude Code)
./grpc-mcp-server --config ./grpc-mcp.yaml

# SSE transport for HTTP clients
./grpc-mcp-server --config ./grpc-mcp.yaml --transport sse --address :8080

# With hot-reload enabled
./grpc-mcp-server --config ./grpc-mcp.yaml --watch
```

## Tool Naming Convention

Tools are named using the format: `{endpoint}.{package.service}.{method}`

Examples:
- `local-api.users.v1.UserService.GetUser`
- `prod.orders.OrderService.CreateOrder`
- `dev.SimpleService.Call`

## Streaming Support

### Server Streaming

Request with single input, receive array of responses:

```json
// Input
{"userId": "123"}

// Output (array of streamed responses)
[
  {"update": "Processing..."},
  {"update": "50% complete"},
  {"result": "Done"}
]
```

### Client Streaming

Send array of requests, receive single response:

```json
// Input (wrapped in requests array)
{
  "requests": [
    {"item": "a"},
    {"item": "b"},
    {"item": "c"}
  ]
}

// Output
{"count": 3, "status": "processed"}
```

### Bidirectional Streaming

Send array of requests, receive array of responses:

```json
// Input
{
  "requests": [
    {"message": "Hello"},
    {"message": "World"}
  ]
}

// Output
[
  {"reply": "Hi there!"},
  {"reply": "Greetings!"}
]
```

## Error Handling

gRPC errors are formatted as MCP tool errors with the format: `[CODE] message`

Examples:
- `[NOT_FOUND] User with ID 123 not found`
- `[PERMISSION_DENIED] Access denied`
- `[INVALID_ARGUMENT] Name field is required`

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     grpc-mcp Server                             │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────────┐    ┌─────────────────────────────────┐   │
│  │  Config Watcher  │───▶│       Tool Registry             │   │
│  │  (fsnotify)      │    │  - Generates MCP tools from     │   │
│  └──────────────────┘    │    discovered gRPC methods      │   │
│           │              │  - Emits list_changed on update │   │
│           ▼              └─────────────────────────────────┘   │
│  ┌──────────────────┐                │                          │
│  │ Endpoint Manager │                ▼                          │
│  │ - gRPC connect   │    ┌─────────────────────────────────┐   │
│  │ - Reflection     │───▶│     MCP Request Handler         │   │
│  │ - Health checks  │    │  - tools/list, tools/call       │   │
│  │ - Auto-reconnect │    │  - JSON ↔ protobuf conversion   │   │
│  └──────────────────┘    └─────────────────────────────────┘   │
│           │                          │                          │
│           ▼                          ▼                          │
│  ┌──────────────────┐    ┌─────────────────────────────────┐   │
│  │ Schema Converter │    │     gRPC Dynamic Invoker        │   │
│  │ Proto → JSON     │    │  - Unary, server/client stream  │   │
│  │ Schema           │    │  - Bidirectional (batch mode)   │   │
│  └──────────────────┘    └─────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## Project Structure

```
grpc-mcp/
├── cmd/
│   └── grpc-mcp-server/
│       └── main.go                 # Entry point, CLI
├── internal/
│   ├── config/
│   │   ├── config.go               # Config types and validation
│   │   ├── loader.go               # YAML parsing, env var expansion
│   │   └── watcher.go              # Hot-reload (fsnotify)
│   ├── grpc/
│   │   ├── client.go               # Connection management
│   │   ├── reflection.go           # Service discovery
│   │   ├── invoker.go              # Dynamic RPC invocation
│   │   └── auth.go                 # Credential providers
│   ├── schema/
│   │   ├── converter.go            # Proto → JSON Schema
│   │   └── types.go                # JSONSchema struct
│   ├── mcp/
│   │   ├── server.go               # MCP server wrapper
│   │   ├── tools.go                # Tool generation
│   │   └── handler.go              # Invocation handler
│   ├── registry/
│   │   ├── registry.go             # Endpoint registry
│   │   └── endpoint.go             # Single endpoint state
│   └── testutil/
│       └── grpcserver.go           # Mock gRPC server for testing
├── pkg/
│   └── naming/
│       └── convention.go           # Tool naming utilities
├── test/
│   └── integration/
│       └── grpc_mcp_test.go        # Integration tests
├── config.example.yaml
├── go.mod
└── README.md
```

## Development

### Prerequisites

- Go 1.22 or later
- A gRPC server with reflection enabled for testing

### Building

```bash
go build -o grpc-mcp-server ./cmd/grpc-mcp-server
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -cover ./...

# Run integration tests only
go test ./test/integration/...

# Run specific package tests
go test ./internal/config/...
```

### Enabling Reflection in Your gRPC Server

For grpc-mcp to discover your services, your gRPC server must have reflection enabled:

**Go:**
```go
import "google.golang.org/grpc/reflection"

server := grpc.NewServer()
// Register your services...
reflection.Register(server)
```

**Python:**
```python
from grpc_reflection.v1alpha import reflection

reflection.enable_server_reflection(SERVICE_NAMES, server)
```

**Node.js:**
```javascript
const grpcReflection = require('grpc-reflection-js');
grpcReflection.addToServer(server);
```

## Type Mappings

| Protobuf Type | JSON Schema Type |
|---------------|------------------|
| double, float | `number` |
| int32, uint32, sint32, fixed32, sfixed32 | `integer` |
| int64, uint64, sint64, fixed64, sfixed64 | `string` (per proto3 JSON spec) |
| bool | `boolean` |
| string | `string` |
| bytes | `string` (format: byte, base64 encoded) |
| enum | `string` with enum values |
| message | `object` with properties |
| repeated | `array` |
| map<K,V> | `object` with additionalProperties |

## Requirements

- gRPC server must have [server reflection](https://github.com/grpc/grpc/blob/master/doc/server-reflection.md) enabled
- For TLS connections, appropriate certificates must be provided
- For mTLS, both client certificate and CA certificate are required

## License

MIT License - see LICENSE file for details.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
