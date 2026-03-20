# net-cat

A TCP group chat server written in Go, inspired by `nc` (NetCat).

The server accepts multiple client connections, prompts each client for a name,
and relays chat messages to everyone else in the room with timestamps.

## Index

- [Features](#features)
- [Project Structure](#project-structure)
- [Requirements](#requirements)
- [Building & Running](#building--running)
- [CLI Options](#cli-options)
- [Connect as a Client](#connect-as-a-client)
- [Logging](#logging)
- [Testing](#testing)
- [Notes on Current Scope](#notes-on-current-scope)

## Features

- TCP chat server with concurrent client handling.
- Default listening port `8989` (custom port supported).
- Welcome banner with Linux ASCII art and name prompt.
- Name validation with retry limit for empty names.
- Message broadcast format:
  - `[YYYY-MM-DD HH:MM:SS][sender]:message`
- Join/leave system notifications to other connected clients.
- Chat history replay for newly joined clients (user messages only).
- Empty user messages are ignored.
- Connection limit control:
  - Default: `10`
  - Custom limit via flag
  - Unlimited mode with negative value
- Optional chat logging to file.

## Project Structure

```text
.
├── go.mod                          # Go module definition and dependency metadata
├── net-cat-README.md               # Original project specification/reference
├── net-cat-audit-README.md         # Audit checklist/reference
├── cmd/
│   └── server/
│       ├── main.go                 # Program entry point, CLI flow, and server startup
│       ├── main_base_test.go       # Core requirement tests
│       └── main_test.go            # Extended/bonus behaviour tests
└── internal/
    ├── server/
    │   ├── config.go               # Runtime configuration model (port, logs, limits, help)
    │   ├── consts.go               # Shared constants (defaults, prompts, banner, limits)
    │   ├── server.go               # TCP listener lifecycle, connection handling, graceful stop
    │   ├── chatroom.go             # Chat room event loop (register, unregister, broadcast, history)
    │   ├── client.go               # Client read/write pumps and connection tear-down
    │   ├── message.go              # Chat and log message formatting with timestamps
    │   └── logger.go               # Buffered async logger for optional file persistence
    └── utils/
        ├── ParseArgs.go            # Command-line argument parsing and validation
        ├── ParseArgs_test.go       # Unit tests for argument parsing
        ├── Atoi.go                 # Custom string-to-int conversion utility
        └── Atoi_test.go            # Unit tests for Atoi utility
```

## Building & Running

### Requirements

- Go 1.24+

### Build the binary

```bash
go build -o TCPChat ./cmd/server
```

### Running

#### Default port (8989)

```bash
./TCPChat ./cmd/server
```

#### Custom port

```bash
./TCPChat ./cmd/server 2525
```

#### With logging and custom connection limit

```bash
./TCPChat ./cmd/server --logfile chat.log --max-connections 32 2525
```

## CLI Options

Usage: `./TCPChat [OPTIONS] [PORT]`

| Option | Argument | Description |
| --- | --- | --- |
| `-h`, `--help` | None | Show help/usage information and exit. |
| `--logfile` | `file name` | Write chat logs to the given file path. |
| `--max-connections` | `integer` | Set maximum concurrent clients. `0` uses default (`10`), negative number means unlimited. |

Notes:

- Passing more than one positional argument returns:
  - `[USAGE]: ./TCPChat $port`

## Connect as a Client

Use `nc` from another terminal:

```bash
nc localhost 8989
```

On connect, the server sends:

1. A Welcome banner.
2. A prompt to enter a name.

After entering a valid name, clients can chat normally.

## Logging

If `--logfile` is set, user messages are written to file in this format:

```text
[YYYY-MM-DD HH:MM:SS][sender][room]:message
```

When `--logfile` is omitted, logs are discarded (no file writes).

## Testing

Run all tests:

```bash
go test ./...
```

Run tests with race detection, randomized execution order, and caching disabled:

```bash
go test -race -shuffle=on -count=1 ./...
```

## Notes on Current Scope

- The server starts with a `general` chat room.
- Command-style chat features (for example runtime room switching or renaming via in-chat commands)
are scaffolded in code but are not fully implemented yet.
