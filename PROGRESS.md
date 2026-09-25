# Ghostwire — Progress Summary & Agent Blueprint

> Dokumen ini dibaca untuk melanjutkan proyek di sesi/agent lain. Isinya: status saat ini,
> keputusan desain, spesifikasi protokol, dan daftar pekerjaan tersisa.
> Update dokumen ini setiap kali ada perubahan besar.

## 1. Ringkasan proyek

**ghostwire** = aplikasi CLI remote desktop untuk Windows, dibangun dengan **Go**
(arsitektur **hexagonal**: ports & adapters), protokol custom mirip RDP bernama **GWRD** over TCP,
viewer berupa **renderer terminal ANSI half-block** (setiap cell = 2 pixel: blok `▀`).

Perintah CLI:
- `ghostwire serve` — sajikan layar mesin ini (capture GDI + input injection SendInput)
- `ghostwire connect <host:port>` — lihat & kendalikan server dari terminal ini
- `ghostwire info <host:port>` — tanya info sesi server (screen/fps/version), ada `--json`

## 2. Status saat ini

| Item | Status |
|---|---|
| Seluruh source Go (30 file) | ✅ selesai |
| `go vet ./...` | ✅ OK |
| `go build ./...` | ✅ OK |
| CLI help (`serve`/`connect`/`info`) | ✅ via cobra |
| Unit/integration test | ❌ belum |
| Docs (`README.md`, `docs/*`) | ❌ belum |
| Smoke test e2e serve+info+auth | ❌ belum |
| Verifikasi akhir (`go test ./...`, e2e viewer TTY) | ❌ belum |

**Perintah verifikasi:** `go vet ./...` → `go test ./...` → `go build ./cmd/ghostwire` → smoke test (§9).

## 3. Stack, tooling, konvensi (WAJIB ikuti)

- Go **1.22+**, module name `ghostwire`.
- Deps runtime: `github.com/spf13/cobra` (CLI), `golang.org/x/sys` (Windows support), `golang.org/x/term` (raw mode TTY).
- Stdlib: `compress/flate`, `crypto/sha256`, `crypto/subtle`, `crypto/tls`, `encoding/binary`, `encoding/json`, `net`, `os`, `syscall`, `unsafe`.
- Scripts: `go build ./cmd/ghostwire`, `go vet ./...`, `go test ./...`
- **Jangan pernah menambah komentar di kode** (kecuali diminta). Dokumentasi = file markdown.
- Logging selalu ke **stderr** (stdout khusus renderer ANSI).
- Bahasa komunikasi user: **Indonesia**.
- Error codes domain (`internal/domain/errors.go`): `protocol, version, auth, busy, timeout, closed, transport, unsupported, config, internal`. Exit code CLI: config→2, auth→3, timeout→4, busy→5, lainnya→1.
- Token env fallback: `GHOSTWIRE_TOKEN` (via `os.Getenv`).

## 4. Peta direktori (semua file)

```
go.mod                                  # module ghostwire, go 1.22.0
cmd/ghostwire/main.go                   # composition root
internal/domain/
  errors.go                             # GhostwireError + ErrorCode
  protocol/constants.go                 # MAGIC GWRD, versi 1, ukuran header/meta, MessageKind, SERVER_VERSION
  protocol/codec.go                     # EncodeMessage/EncodeJSONMessage, MessageDecoder (feed fragment-safe)
  protocol/messages.go                  # JSON control: Hello/Welcome/Ping/Error/Bye/ScreenInfo (+validasi)
  protocol/frame.go                     # FrameMeta 32B encode/decode
  input/events.go                       # Key/Modifier/MouseButton, InputEvent, codec 20B
  screen/frame.go                       # ScreenFrame, ScaleFrameNearest, SamePixels
internal/port/
  server.go                             # ServerPort, ServeOptions, ConnectOptions, TlsOptions
  viewer.go                             # ViewerPort, SessionPort, SessionEvent, SessionInfo, SessionStats
  transport.go                          # Conn, Listener, TransportPort
  capture.go                            # ScreenCapturerPort, CaptureOptions
  inject.go                             # InputInjectorPort
  compress.go                           # CompressorPort
  logger.go                             # Logger, LogLevel, IsLevelEnabled
internal/service/
  server.go                             # ServerService (handshake/auth, capture loop, keepalive, backpressure)
  client.go                             # ClientService (connect, inbox pump, queue drop-4, stats rtt/fps)
internal/util/asyncqueue.go             # AsyncQueue generic (channel-based)
internal/adapter/
  cli/cli.go                            # cobra CLI: serve/connect/info + validation
  terminal/input_parser.go              # ParseTerminalInput, KeyFromChar, MapCellToScreen
  terminal/ansi_renderer.go             # AnsiRenderer diff-based, status line
  terminal/viewer.go                    # TerminalViewer (raw mode, flush ESC 50ms, Ctrl+Q, resize)
  transport/tcp.go                      # parseAddress, TCP + TLS, connect timeout 10s
  capture/gdi.go                        # GDI capturer (stub/scaffold, build tag)
  capture/gdi_other.go                  # non-Windows stub (returns unsupported)
  inject/sendinput_windows.go           # SendInput via syscall (INPUT struct 28B)
  inject/sendinput_other.go             # non-Windows stub (returns unsupported)
  compress/deflate.go                   # compress/flate level 1
  compress/null.go                      # name 'none'
  logging/console.go                    # ConsoleLogger(level, sink=stderr)
test/                                   # ❌ belum dibuat
```

Arah dependensi: **domain ← ports ← services ← adapters; main.go menyusun semuanya.**
Adapter primary TIDAK boleh di-import oleh core/services (hanya sebaliknya).

## 5. Arsitektur & alur kerja

- **Server** (`ServerService.Serve(opts, ctx)`): `capturer.Start()` → `transport.Listen()` →
  ticker `time.NewTicker(1s/fps)` → grab → skip bila piksel sama →
  deflate → tulis frame. Berhenti & cleanup saat `ctx` cancelled (kirim Bye → tutup listener →
  `capturer.Close()` → `injector.Close()`).
- **Handshake**: client kirim `Hello{ProtocolVersion, ClientName, Token}` → server balas
  `Welcome{Ok:true,ServerVersion,Screen,Fps}` atau `Welcome{Ok:false,Reason:'unauthorized'}` + tutup.
  Client kedua saat sesi aktif terima `Error{Code:'busy'}` + tutup. Timeout handshake 10s (fix).
- **Auth**: `crypto/sha256` + `crypto/subtle.ConstantTimeCompare`. Token wajib non-empty.
- **Keepalive**: keduanya kirim `Ping{T}` tiap `pingIntervalMs` (default 5000), tutup sesi bila
  `pingTimeoutMs` (default 15000) tidak ada aktivitas; Pong dipakai hitung `rttMs`.
- **Backpressure**: `Conn.Write()` → boolean; server set `active.paused = !flushed`, resume via
  `OnDrain`. Client: inbox slice + `pump()` goroutine sekuensial; queue event dibatasi 4, frame tertua
  di-drop (`framesDropped++`).
- **Viewer**: `TerminalViewer.Run(ctx, opts)` — cek TTY dulu (error `unsupported`),
  connect, `renderer.Begin()` (hide cursor, no-wrap, mouse 1000/1002/1003/1006, clear), raw mode via
  `golang.org/x/term.MakeRaw`, goroutine baca stdin → parser → `sendInput`; cleanup di
  `defer` (`term.Restore`, `renderer.End()`).

## 6. Spesifikasi protokol GWRD (ringkas, versi 1)

**Header 12 byte, big-endian:**
`magic u32=0x47575244('GWRD') | version u8=1 | kind u8 | flags u16 | payloadLen u32`
- `flags` bit0 = Deflate (untuk payload JSON kontrol).
- `MAX_PAYLOAD_SIZE` = 32 MB; decoder melempar `protocol` bila magic/version salah.

**MessageKind:** Hello=1, Welcome=2, Frame=3, Input=4, Ping=5, Pong=6, Error=7, Bye=8, ScreenInfo=9.

**Frame** = `FrameMeta 32B + payload piksel`:
`seq u32 | timestampMs u64 | x u16 | y u16 | w u16 | h u16 | pixelFormat u8(0=BGRA8) | encoding u8(0=Raw,1=Deflate) | rawLength u32 | reserved`.
`rawLength` WAJIB = w*h*4 (divalidasi decoder). `MAX_FRAME_RAW_SIZE` = 64 MB.

**Input event = 20 byte:** `kind u8 (Key=1,MouseMove=2,MouseButton=3,Wheel=4,Text=5) | flags u8(bit0=down) | modifiers u16 | code u16 | pad u16 | x i32 | y i32 | deltaY i16 | pad i16`.
- Key.code = **VK Windows**; modifiers bitmask `Shift=1,Ctrl=2,Alt=4,Meta=8`.
- MouseButton: `0=Left,1=Right,2=Middle,3=X1,4=X2`.

**ScreenInfo** `{width,height}` dikirim server saat resolusi berubah.

## 7. Detail implementasi kunci

- **Pixel format = BGRA** (offset 0=B,1=G,2=R,3=A). Renderer membaca begitu.
- **Renderer**: tiap cell = box-average 2 blok vertikal piksel → fg (atas) + bg (bawah) + `▀`.
  Diff per cell terhadap buffer sebelumnya; SGR 24-bit hanya saat warna berubah; cursor move per run.
  Baris terakhir = status line (`viewportRows = rows-1`). `Resize()` → clear + repaint penuh.
- **Peta mouse** (`MapCellToScreen`): col/row 1-based di-clamp; `x=(col-1)*W/cols`,
  `y=(2*row-1)*H/(2*rows)` (setara pusat 2 pixel vertikal), hasil di-clamp ke layar.
- **Parser input** menangani: printable ASCII→VK (tabel shift/symbol US), Ctrl+letter (0x01–0x1A),
  Enter/Tab/Backspace(0x7f & 0x08)/NUL=Ctrl+Space, CSI (`A-D/H/F/Z`, `~` 1–24 termasuk F1–F12,
  modifier `1;Nm`), SS3 (`OP`–`OS` F1–F4, `OH/OF` Home/End), mouse SGR `<b;x;y(M|m)` (+wheel 64/65,
  motion 32), bracketed paste `200~…201~`, Alt+char (`ESC`+byte), focus `CSI I/O` diabaikan.
  Byte ESC tergantung → **flush 50ms** → Escape/abaikan.
- **Injector**: sinkronisasi modifier (mouse→tekan yang kurang; key down→modifier dulu; key up→
  lepas tombol lalu **lepas semua modifier**); `releaseAll()` saat sesi tutup; teks →
  `KEYEVENTF_UNICODE`; mouse absolut di-normalisasi ke primary screen (`GetSystemMetrics(0/1)`).
- **GDI capturer**: Windows-only via `syscall.NewLazyDLL` (bukan `golang.org/x/sys/windows`).
  DC/bitmap dibuat sekali (`Start`), `StretchBlt` downscale ke `maxWidth`, **2 buffer piksel bergantian**.
  Non-Windows = stub返回 `ErrUnsupported`.
- **TLS**: stdlib `crypto/tls` (wraps SChannel di Windows).
- **Input FFI**: manual syscall `user32.dll` `SendInput`, `INPUT` struct 28 bytes (compile-time size assertion belum, TODO).

## 8. Pekerjaan tersisa (urutan pengerjaan)

1. **Buat test** — `go test ./...` (port semua test TS yang sudah ada):
   - `internal/domain/protocol/codec_test.go`: roundtrip message, reassemble byte-by-byte, bad magic, unsupported version, transparent compression.
   - `internal/domain/protocol/messages_test.go`: roundtrip hello/welcome/ping/error/bye/screeninfo, reject invalid types/bad screen, reject non-object.
   - `internal/domain/protocol/frame_test.go`: roundtrip, reject bad pixel format, reject raw length mismatch, reject truncated.
   - `internal/domain/input/events_test.go`: roundtrip all event kinds, reject wrong length, reject unknown kind.
   - `internal/adapter/terminal/input_parser_test.go`: plain/uppercase/symbol, Ctrl chord, Enter/Tab/BS, CSI arrow+modifier, tilde, SS3, SGR mouse, Alt+char, MapCellToScreen (clamping).
   - `internal/adapter/transport/tcp_test.go`: parseAddress (valid, IPv6, no colon, bad port).
   - `internal/util/asyncqueue_test.go`: FIFO, End(), RemoveFirst.
   - `internal/service/session_test.go`: handshake ok, frame masuk, sendInput, token salah → auth, client kedua → busy, abort → shutdown.
   - `internal/adapter/cli/cli_test.go`: validation errors (config code 2), mapping flags.
2. **GDI + SendInput Windows**: implementasi aktual via syscall (saat ini stub/scaffold).
3. **GDI + SendInput compile-time size assertion**: `var _ [28]byte = [unsafe.Sizeof(winInput{})]byte{}`
4. **Docs**: `README.md` (instalasi, contoh perintah serve/connect/info, flags, catatan Windows, troubleshooting TTY).
5. **Verifikasi akhir**: `go vet ./...` → `go test ./...` → `go build ./cmd/ghostwire` → smoke test §9 →
   e2e viewer sungguhan di terminal TTY (buka 2 terminal: serve + connect, gerakkan mouse, ketik, `Ctrl+Q` untuk keluar).

## 9. Smoke test e2e (rencana)

```bash
go build -o ghostwire.exe ./cmd/ghostwire
./ghostwire.exe serve --token test123 --address 127.0.0.1:59010 --fps 5 --log-level debug  # terminal 1
./ghostwire.exe info 127.0.0.1:59010 --token test123                                       # → screen/fps/server, exit 0
./ghostwire.exe info 127.0.0.1:59010 --token wrong                                          # → "error: unauthorized", exit 3
```

## 10. Catatan pitfalls

- Jangan pakai `fmt.Fprint(os.Stdout, ...)` selain dari renderer/logger di viewer (log = stderr).
- `Serve()` menunggu `ctx` cancelled — test harus `cancel()` + `await` atau test hang.
- Client busy membuang error `GhostwireError{Code:'protocol', Message:'busy: ...'}` — sesuaikan ekspektasi test.
- Frame identik dilewati server (`SamePixels`) — FakeCapturer harus mengubah piksel tiap `Grab()`.
- Keyboard shortcut viewer: **Ctrl+Q keluar** (Ctrl+Q TIDAK diteruskan ke remote); Ctrl+C remote
  diteruskan normal (raw mode mematikan ISIG).
- Info command sengaja tidak punya flag `--ping-*` (di-hardcode 5000/15000).
- Non-Windows: GDI capturer dan SendInput injector mengembalikan `ErrUnsupported`.
- `golang.org/x/sys/windows` TIDAK expose GDI/SendInput — harus manual `syscall.NewLazyDLL`.
