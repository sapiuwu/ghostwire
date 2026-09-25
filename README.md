# Ghostwire

Remote desktop CLI untuk Windows. Menggunakan protokol custom bernama **GWRD** over TCP, dengan renderer terminal ANSI half-block (`▀`) yang memungkinkan Anda melihat dan mengontrol layar Windows dari terminal mana pun.

## Features

- **Remote desktop via terminal** — Lihat layar Windows dalam terminal dengan renderer ANSI 24-bit color
- **Full input injection** — Keyboard, mouse, scroll, text input (Unicode) melalui Windows SendInput API
- **Custom protocol (GWRD)** — Binary protocol ringan, kompatibel dengan RDP tapi lebih sederhana
- **TLS support** — Enkripsi koneksi via TLS (SChannel di Windows)
- **Deflate compression** — Frame kompresi real-time dengan `compress/flate` level 1
- **Backpressure handling** — Server pause/resume jika client tidak bisa mengikuti
- **Keepalive & timeout** — Deteksi koneksi mati, reconnect otomatis
- **Single binary** — Satu executable, zero runtime dependencies

## Quick Start

```bash
# Build
go build -o ghostwire.exe ./cmd/ghostwire

# Terminal 1: Start server (share layar)
./ghostwire.exe serve --token mysecret --address 0.0.0.0:5901

# Terminal 2: Connect (lihat & kontrol)
./ghostwire.exe connect 127.0.0.1:5901 --token mysecret

# Terminal 2: Query info saja (tanpa viewer)
./ghostwire.exe info 127.0.0.1:5901 --token mysecret
```

## Commands

### `ghostwire serve`

Menyajikan layar mesin ini ke client yang terhubung.

```bash
ghostwire serve [flags]
```

| Flag | Default | Deskripsi |
|---|---|---|
| `-a, --address` | `0.0.0.0:5901` | Listen address (host:port) |
| `-t, --token` | _(required)_ | Auth token (env: `GHOSTWIRE_TOKEN`) |
| `--fps` | `10` | Capture frame rate (1-60) |
| `--max-width` | `1600` | Max frame width dalam pixels (0 = native) |
| `--compress` | `deflate` | Frame compression (`deflate` atau `none`) |
| `--tls-cert` | - | TLS certificate path (PEM) |
| `--tls-key` | - | TLS private key path (PEM) |
| `--ping-interval` | `5000` | Keepalive ping interval (ms) |
| `--ping-timeout` | `15000` | Keepalive timeout (ms) |
| `--log-level` | `info` | Log verbosity (`debug`, `info`, `warn`, `error`) |

**Contoh:**
```bash
ghostwire serve --token secret123 --fps 15 --max-width 1280
ghostwire serve --token secret123 --tls-cert cert.pem --tls-key key.pem
GHOSTWIRE_TOKEN=secret123 ghostwire serve
```

### `ghostwire connect <host:port>`

Terhubung ke server ghostwire, menampilkan layar di terminal, dan mengontrol remote machine.

```bash
ghostwire connect <host:port> [flags]
```

| Flag | Default | Deskripsi |
|---|---|---|
| `-t, --token` | _(required)_ | Auth token (env: `GHOSTWIRE_TOKEN`) |
| `-n, --name` | `ghostwire-cli` | Client name yang ditampilkan ke server |
| `--tls` | `false` | Gunakan TLS untuk koneksi |
| `--ca-cert` | - | Custom CA certificate (PEM) |
| `--insecure` | `false` | Skip server certificate verification |
| `--ping-interval` | `5000` | Keepalive ping interval (ms) |
| `--ping-timeout` | `15000` | Keepalive timeout (ms) |
| `--log-level` | `info` | Log verbosity |

**Keyboard shortcuts (viewer):**
- `Ctrl+Q` — Keluar dari session
- Semua input lainnya diteruskan ke remote machine

### `ghostwire info <host:port>`

Query session info dari server tanpa membuka viewer.

```bash
ghostwire info <host:port> [flags]
```

| Flag | Default | Deskripsi |
|---|---|---|
| `-t, --token` | _(required)_ | Auth token (env: `GHOSTWIRE_TOKEN`) |
| `--tls` | `false` | Gunakan TLS |
| `--ca-cert` | - | Custom CA certificate (PEM) |
| `--insecure` | `false` | Skip certificate verification |
| `--json` | `false` | Print raw JSON output |

**Output:**
```
address: 127.0.0.1:5901
screen:  1920x1080
fps:     10
server:  0.1.0
```

## Architecture

Ghostwire menggunakan **hexagonal architecture** (ports & adapters). Dependency flow:

```
domain ← ports ← services ← adapters ← main.go
```

### Directory Structure

```
cmd/ghostwire/main.go                  # Composition root
internal/
  domain/
    errors.go                          # GhostwireError + ErrorCode
    protocol/
      constants.go                     # GWRD protocol constants
      codec.go                         # MessageEncoder/Decoder (fragment-safe)
      messages.go                      # JSON control messages
      frame.go                         # FrameMeta 32-byte encode/decode
    input/events.go                    # Key/Modifier/MouseButton + 20-byte codec
    screen/frame.go                    # ScreenFrame, scaling, pixel comparison
  port/
    server.go                          # ServerPort interface
    viewer.go                          # ViewerPort, SessionPort interfaces
    transport.go                       # Conn, Listener, TransportPort
    capture.go                         # ScreenCapturerPort
    inject.go                          # InputInjectorPort
    compress.go                        # CompressorPort
    logger.go                          # Logger interface
  service/
    server.go                          # ServerService (handshake, capture loop, keepalive)
    client.go                          # ClientService (connect, inbox pump, session)
  util/asyncqueue.go                   # AsyncQueue (generic, channel-based)
  adapter/
    cli/cli.go                         # Cobra CLI (serve/connect/info)
    terminal/
      input_parser.go                  # Terminal input parser (CSI, SS3, SGR mouse, etc.)
      ansi_renderer.go                 # Diff-based ANSI renderer
      viewer.go                        # Terminal viewer driver (raw mode, resize)
    transport/tcp.go                   # TCP + TLS transport
    capture/
      gdi.go                           # GDI capturer (Windows, syscall)
      gdi_other.go                     # Non-Windows stub
    inject/
      sendinput_windows.go             # SendInput (Windows, syscall)
      sendinput_other.go               # Non-Windows stub
    compress/
      deflate.go                       # compress/flate wrapper
      null.go                          # No-op compressor
    logging/console.go                 # stderr logger
```

### Hexagonal Architecture Rules

- **Domain** — Pure business logic, zero dependencies
- **Ports** — Interfaces that define contracts
- **Services** — Business logic using ports
- **Adapters** — Implementations of ports (terminal, TCP, GDI, SendInput)
- **main.go** — Composition root, wires everything together

Primary adapters (CLI, terminal) must NOT be imported by core/services.

## Protocol (GWRD)

Ghostwire uses a custom binary protocol called GWRD over TCP.

### Message Format

Every message starts with a **12-byte header** (big-endian):

```
Offset  Size  Field
0       4     Magic (0x47575244 = "GWRD")
4       1     Version (1)
5       1     MessageKind
6       2     Flags (bit 0 = Deflate)
8       4     Payload length
12      N     Payload
```

### Message Kinds

| Kind | Value | Direction | Payload |
|---|---|---|---|
| Hello | 1 | Client → Server | JSON: `{protocolVersion, clientName, token}` |
| Welcome | 2 | Server → Client | JSON: `{ok, reason?, serverVersion?, screen?, fps?}` |
| Frame | 3 | Server → Client | FrameMeta (32B) + pixel data |
| Input | 4 | Client → Server | InputEvent (20B) |
| Ping | 5 | Both | JSON: `{t}` |
| Pong | 6 | Both | JSON: `{t}` |
| Error | 7 | Both | JSON: `{code, message}` |
| Bye | 8 | Both | JSON: `{reason}` |
| ScreenInfo | 9 | Server → Client | JSON: `{width, height}` |

### Frame Format

Frame payload = **FrameMeta 32 bytes** + compressed/raw pixel data:

```
Offset  Size  Field
0       4     Sequence number
4       8     Timestamp (ms)
12      2     X
14      2     Y
16      2     Width
18      2     Height
20      1     Pixel format (0 = BGRA8)
21      1     Encoding (0 = Raw, 1 = Deflate)
22      4     Raw length (must = width × height × 4)
26      6     Reserved
```

Pixel format is **BGRA** (Blue, Green, Red, Alpha — 4 bytes per pixel).

### Input Event Format

Input event = **20 bytes** (little-endian):

```
Offset  Size  Field
0       1     Kind (1=Key, 2=MouseMove, 3=MouseButton, 4=Wheel, 5=Text)
1       1     Flags (bit 0 = key/button down)
2       2     Modifiers (bitmask: Shift=1, Ctrl=2, Alt=4, Meta=8)
4       2     Code (VK code for Key, button index for Mouse)
6       2     Reserved
8       4     X (signed, for mouse events)
12      4     Y (signed, for mouse events)
16      2     DeltaY (signed, for wheel events)
18      2     Reserved
```

### Connection Flow

```
Client                          Server
  |                               |
  |--- Hello {token} ----------->|
  |                               | verify token (SHA-256 + timingSafeEqual)
  |<-- Welcome {screen,fps} -----|
  |                               |
  |<-- Frame {meta + pixels} ----|  (repeating)
  |--- Input {event} ----------->|  (repeating)
  |                               |
  |<-- Ping {t} -----------------|  (every 5s)
  |--- Pong {t} ---------------->|  (every 5s)
  |                               |
  |--- Bye {reason} ------------>|  (on Ctrl+Q)
  |<-- Bye {reason} -------------|  (on shutdown)
```

### Limits

| Parameter | Value |
|---|---|
| Max payload size | 32 MB |
| Max frame raw size | 64 MB |
| Handshake timeout | 10 seconds |
| Default ping interval | 5 seconds |
| Default ping timeout | 15 seconds |

## Build

### Requirements

- Go 1.22+
- Windows (for GDI capture + SendInput — non-Windows returns `unsupported`)

### Build

```bash
go build -o ghostwire.exe ./cmd/ghostwire
```

### Cross-compile for Windows

```bash
GOOS=windows GOARCH=amd64 go build -o ghostwire.exe ./cmd/ghostwire
```

### Test

```bash
go vet ./...
go test ./...
```

### Run (development)

```bash
go run ./cmd/ghostwire serve --token test
go run ./cmd/ghostwire connect 127.0.0.1:5901 --token test
```

## Environment Variables

| Variable | Description |
|---|---|
| `GHOSTWIRE_TOKEN` | Default auth token (fallback for `--token`) |

## Exit Codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error |
| 2 | Config error (invalid flags) |
| 3 | Auth error (unauthorized) |
| 4 | Timeout error |
| 5 | Busy (another viewer connected) |

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `golang.org/x/term` | Terminal raw mode, TTY detection |
| `golang.org/x/sys` | Windows system calls |

Stdlib only (no external deps):
- `compress/flate` — Deflate compression
- `crypto/sha256` — Token hashing
- `crypto/subtle` — Timing-safe comparison
- `crypto/tls` — TLS support
- `encoding/binary` — Binary protocol encoding
- `encoding/json` — Control message serialization
- `syscall` + `unsafe` — Windows FFI (GDI, SendInput)

## Security

- Token di-hash dengan SHA-256 sebelum dibandingkan
- Perbandingan token menggunakan `crypto/subtle.ConstantTimeCompare` (timing-safe)
- TLS opsional, bisa self-signed dengan `--insecure`
- Token tidak pernah di-log

## Platform Support

| Platform | Status |
|---|---|
| Windows (amd64) | Full support (GDI capture + SendInput) |
| Linux | Partial (connect/info work, capture/inject unsupported) |
| macOS | Partial (connect/info work, capture/inject unsupported) |

## License

MIT
