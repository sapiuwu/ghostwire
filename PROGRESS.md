# Ghostwire — Progress Summary & Agent Blueprint

> Doc ini dibaca untuk melanjutkan proyek di sesi/agent lain. Isinya: status saat ini,
> keputusan desain, spesifikasi protokol, dan daftar pekerjaan tersisa.
> Update dokumen ini setiap kali ada perubahan besar.

## 1. Ringkasan proyek

**ghostwire** = aplikasi CLI remote desktop untuk Windows, dibangun dengan **Node.js + TypeScript**
(arsitektur **hexagonal**: ports & adapters), protokol custom miri­p RDP bernama **GWRD** over TCP,
viewer berupa **renderer terminal ANSI half-block** (setiap cell = 2 pixel: blok `▀`).

Perintah CLI:
- `ghostwire serve` — sajikan layar mesin ini (capture GDI + input injection SendInput)
- `ghostwire connect <host:port>` — lihat & kendalikan server dari terminal ini
- `ghostwire info <host:port>` — tanya info sesi server (screen/fps/version), ada `--json`

## 2. Status saat ini (per 25 Sep 2026)

| Item | Status |
|---|---|
| Seluruh source `src/**` (29 file) | ✅ selesai & typecheck lolos |
| `npx tsc -p tsconfig.test.json` (typecheck) | ✅ OK |
| `npm run build` (tsc → `dist/`) | ✅ OK |
| CLI help (`serve`/`connect`/`info`) | ✅ teruji |
| Smoke test e2e serve+info+auth | ✅ teruji (lihat §9) |
| Unit/integration test | ⚠️ separuh jalan: `test/helpers.ts`, `test/protocol.test.ts`, `test/input.test.ts` sudah ditulis **belum dijalankan**; 6 file test lagi belum dibuat (lihat §8) |
| Docs (`README.md`, `docs/*`) | ❌ belum |
| Verifikasi akhir (`npm test`, e2e viewer TTY) | ❌ belum |

**Perintah verifikasi:** `npx tsc -p tsconfig.test.json` → `npm test` → `npm run build` → smoke test (§9).

## 3. Stack, tooling, konvensi (WAJIB ikuti)

- Node **v22.18.0**, `"type": "module"` (ESM), TypeScript **strict** + `NodeNext` + `verbatimModuleSyntax` + `exactOptionalPropertyTypes` + `noUncheckedIndexedAccess`.
- Import antar-file **wajib berekstensi `.js`** (meski file sumber `.ts`).
- Deps runtime: `commander@^14` (CLI), `koffi@^3` (FFI Win32). Dev: `typescript@^5.8`, `vitest@^3.2`, `@types/node@^22`, `tsx@^4`.
- Scripts package.json: `build` (tsc), `typecheck` (tsc -p tsconfig.test.json), `test` (vitest run), `dev` (tsx src/main.ts), `start`.
- **Jangan pernah menambah komentar di kode** (kecuali diminta). Dokumentasi = file markdown.
- Logging selalu ke **stderr** (stdout khusus renderer ANSI).
- `rg` TIDAK tersedia di mesin ini — pakai tool Grep / `Select-String`.
- Bahasa komunikasi user: **Indonesia**.
- Error codes domain (`src/core/domain/errors.ts`): `protocol, version, auth, busy, timeout, closed, transport, unsupported, config, internal`. Exit code CLI: config→2, auth→3, timeout→4, busy→5, lainnya→1.
- Token env fallback: `GHOSTWIRE_TOKEN` (via `Option.env()` commander).

## 4. Peta direktori (semua file ada, kecuali ditandai ❌)

```
src/main.ts                          # composition root (#!/usr/bin/env node, top-level await runCli)
src/core/domain/
  errors.ts                          # GhostwireError + ErrorCode (+ 'config')
  protocol/constants.ts              # MAGIC GWRD, versi 1, ukuran header/meta, MessageKind, SERVER_VERSION
  protocol/codec.ts                  # encodeMessage/encodeJSONMessage, MessageDecoder (feed fragment-safe)
  protocol/messages.ts               # JSON control: Hello/Welcome/Ping/Error/Bye/ScreenInfo (+validasi ketat)
  protocol/frame.ts                  # FrameMeta 32B encode/decode (validasi format/dimensi/rawLength)
  input/events.ts                    # Key(VK Windows)/Modifier/MouseButton, InputEvent, codec 20B
  screen/frame.ts                    # ScreenFrame {width,height,format:'bgra8',timestampMs,data:Uint8Array},
                                     # scaleFrameNearest, samePixels
  session/config.ts                  # ServeOptions/ConnectOptions/Tls*/SessionInfo/SessionStats
src/core/ports/
  inbound/server.ts                  # ServerPort.serve(options, signal), ServerStats
  inbound/viewer.ts                  # ViewerPort.connect, SessionPort{info,events,sendInput,stats,close}, SessionEvent
  outbound/{transport,capture,inject,compress,logger}.ts   # Conn/Listener/TransportPort, dst.
src/core/services/
  server.service.ts                  # handshake/auth, capture loop, keepalive, backpressure, satu client
  client.service.ts                  # handshake, inbox pump sekuensial, queue drop-4, stats rtt/fps
src/core/util/async-queue.ts         # AsyncQueue + removeFirst (dipakai untuk drop frame)
src/adapters/primary/
  cli/index.ts                       # createCli(handlers, signal) + runCli(argv, handlers) -> exit code
  terminal/input-parser.ts           # parseTerminalInput + keyFromChar + mapCellToScreen (pure, testable)
  terminal/ansi-renderer.ts          # AnsiRenderer diff-based, status line, viewportRows getter
  terminal/terminal-viewer.ts        # driver TTY: raw mode, flush ESC 50ms, Ctrl+Q, resize, cleanup
src/adapters/secondary/
  transport/tcp-transport.ts         # parseAddress, TCP + TLS, connect timeout 10s, close linger 2s
  capture/gdi-capturer.ts            # koffi GDI: DC persisten, StretchBlt downscale maxWidth, buffer ganda
  inject/sendinput-injector.ts       # koffi SendInput, state machine modifier, text UNICODE, mouse absolut
  compress/deflate-compressor.ts     # deflate level 1 async, inflate dengan maxOutputLength guard
  compress/null-compressor.ts        # name 'none', decompress selalu reject
  logging/console-logger.ts          # ConsoleLogger(level, sink=stderr)
test/helpers.ts                      # MemoryTransport/Conn, FakeCapturer, FakeInjector, TestLogger, waitFor ✅ditulis
test/protocol.test.ts                # codec/messages/frame-meta ✅ditulis (belum dijalankan)
test/input.test.ts                   # input codec + keyFromChar ✅ditulis (belum dijalankan)
# ❌ belum dibuat: input-parser, renderer, session, cli, transport, async-queue tests
```

Arah dependensi: **domain ← ports ← services ← adapters; main.ts menyusun semuanya.**
Adapter primary TIDAK boleh di-import oleh core/services (hanya sebaliknya).

## 5. Arsitektur & alur kerja

- **Server** (`ServerService.serve(options, signal)`): `capturer.start()` → `transport.listen()` →
  loop `setTimeout` (hanya 1 tick dalam volu; fps = interval) → grab → skip bila piksel sama →
  deflate → tulis frame. Berhenti & cleanup saat `signal` abort (kirim Bye → tutup listener →
  `capturer.close()` → `injector.close()`).
- **Handshake**: client kirim `Hello{protocolVersion, clientName, token}` → server balas
  `Welcome{ok:true,serverVersion,screen,fps}` atau `Welcome{ok:false,reason:'unauthorized'}` + tutup.
  Client kedua saat sesi aktif terima `Error{code:'busy'}` + tutup. Timeout handshake 10s (fix).
- **Auth**: sha256 + `timingSafeEqual`. Token wajib non-empty (divalidasi CLI & service).
- **Keepalive**: keduanya kirim `Ping{t}` tiap `pingIntervalMs` (default 5000), tutup sesi bila
  `pingTimeoutMs` (default 15000) tidak ada aktivitas; Pong dipakai hitung `rttMs`.
- **Backpressure**: `Conn.write()` → boolean; server set `active.paused = !flushed`, resume via
  `onDrain`. Client: inbox array + `pump()` async sekuensial (urutan frame terjaga lewat inflate);
  queue event dibatasi 4, frame tertua di-drop (`framesDropped++`).
- **Viewer**: `TerminalViewer.run(connectOptions, signal)` — cek TTY dulu (error `unsupported`),
  connect, `renderer.begin()` (hide cursor, no-wrap, mouse 1000/1002/1003/1006, clear), raw mode,
  loop `for await (session.events())` → render + status tiap 500ms; stdin → parser → `sendInput`;
  cleanup di `finally` (restore raw, `renderer.end()`, lepas listener).

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
- MouseButton: `0=Left,1=Right,2=Middle,3=X1,4=X2` (xterm SGR 0/1/2 → 0/2/1 di-warnai parser).
- Text.code = UTF-16 code unit (surrogate dipisah jadi 2 event).

**ScreenInfo** `{width,height}` dikirim server saat resolusi berubah.

## 7. Detail implementasi kunci (jangan diubah sembarangan)

- **Pixel format = BGRA** (offset 0=B,1=G,2=R,3=A). Renderer membaca begitu.
- **Renderer**: tiap cell = box-average 2 blok vertikal piksel → fg (atas) + bg (bawah) + `▀`.
  Diff per cell terhadap buffer sebelumnya; SGR 24-bit hanya saat warna berubah; cursor move per run.
  Baris terakhir = status line (`viewportRows = rows-1`). `resize()` → clear + repaint penuh.
- **Peta mouse** (`mapCellToScreen`): col/row 1-based di-clamp; `x=floor((col-0.5)*W/cols)`,
  `y=floor((2*row-0.5)*H/(2*rows))` (setara pusat 2 pixel vertikal), hasil di-clamp ke layar.
- **Parser input** menangani: printable ASCII→VK (tabel shift/symbol US), Ctrl+letter (0x01–0x1A),
  Enter/Tab/Backspace(0x7f & 0x08)/NUL=Ctrl+Space, CSI (`A-D/H/F/Z`, `~` 1–24 termasuk F1–F12,
  modifier `1;Nm`), SS3 (`OP`–`OS` F1–F4, `OH/OF` Home/End), mouse SGR `<b;x;y(M|m)` (+wheel 64/65,
  motion 32), bracketed paste `200~…201~`, Alt+char (`ESC`+byte), focus `CSI I/O` diabaikan,
  OSC/DCS dikonsumsi, UTF-8 → event `text`. Byte ESC tergantung → **flush 50ms** → Escape/abaikan.
  `flush=true` menyelesaikan sekuens parsial; tanpa flush, sekuens parsial → `consumed` berhenti.
- **Injector**: sinkronisasi modifier (mouse→tekan yang kurang; key down→modifier dulu; key up→
  lepas tombol lalu **lepas semua modifier**); `releaseAll()` saat sesi tutup; teks →
  `KEYEVENTF_UNICODE`; mouse absolut di-normalisasi ke primary screen (`GetSystemMetrics(0/1)`).
  X1/X2 pakai `mouseData=XBUTTON1/2` (sudah diperbaiki).
- **GDI capturer**: DC/bitmap dibuat sekali (`start`), `StretchBlt` downscale ke `maxWidth`
  (0=native), **2 buffer piksel bergantian** (aman terhadap `lastPixels` & kompresi in-flight).
  Benchmark terukur: BitBlt fullscreen 17.7ms, GetDIBits 0.95ms — jangan ganti pendekatan.
- **koffi**: pointer = BigInt, `null` untuk NULL, Buffer/TypedArray ok untuk pointer arg,
  **`koffi.view` tidak andal — jangan dipakai**. Struct INPUT/union sudah didefinisikan di
  `sendinput-injector.ts`.
- **TLS** sudah didukung transport (`--tls-cert/--tls-key` di serve; `--tls/--ca-cert/--insecure`
  di connect/info).

## 8. Pekerjaan tersisa (urutan pengerjaan)

1. **Jalankan & perbaiki test yang sudah ditulis** — `npm test` (pastikan vitest resolve import
   `.js` → `.ts`; kalau belum, tambah `vitest.config.ts` dengan alias/resolve).
2. **Buat test yang belum ada** (rencana per file):
   - `test/input-parser.test.ts`: plain/uppercase/symbol, Ctrl chord (0x03→Ctrl+C), Enter/Tab/BS,
     CSI arrow+modifier (`ESC[1;5A`→Ctrl+Up), tilde (Delete/F5), SS3 (F1), SGR mouse press/release/
     motion/wheel/modifier + pemakaian `mapCell`, Alt+char, bracketed paste, incomplete ESC flush,
     UTF-8 → text, `mapCellToScreen` (clamping, layar 0, nilai tengah).
   - `test/renderer.test.ts`: io palsu menampung output; frame pertama → paint penuh (cursor + SGR),
     frame identik → output kosong, sel berubah → hanya sel itu, `status()` di baris terakhir +
     reset SGR state, `resize()` → clear.
   - `test/session.test.ts` (integrasi ServerService+ClientService via `MemoryTransport`):
     handshake ok (info.screen sesuai FakeCapturer 8x4), frame masuk (`waitFor` + cek ukuran),
     `sendInput` → `FakeInjector.events`, token salah → reject `auth`, client kedua → `busy:`,
     `controller.abort()` → serve selesai, `capturer.closed` & `injector.closed` true,
     client terima `closed`/`server closed session` saat server shutdown.
   - `test/cli.test.ts`: `createCli` + `program.parseAsync` dengan handler mock: mapping opsi serve
     (default fps 10, maxWidth 1600, compress deflate, tls hanya bila cert+key), error validasi
     (`config`, pesan berisi `--fps` dll), mapping connect (`address`, `tls: {insecure:true}`).
     Pakai `program.exitOverride()` bila perlu; jangan uji `--help` (menulis stdout & exit).
   - `test/transport.test.ts`: `parseAddress` (`host:port`, `127.0.0.1:5901`, `[::1]:5901`,
     tanpa `:` → throw, port abc → throw).
   - `test/async-queue.test.ts`: urutan FIFO, `end()` → iterator selesai, `end(err)` → throw,
     `removeFirst` menghapus sesuai predicate.
3. **Docs**: `README.md` (instalasi, contoh perintah serve/connect/info, flags, catatan Windows,
   troubleshooting TTY), `docs/architecture.md` (peta layer, alur data, diagram dependensi),
   `docs/protocol.md` (tabel §6 lengkap: header, kind, meta, input, state machine handshake).
4. **Verifikasi akhir**: `npm run typecheck` → `npm test` → `npm run build` → smoke test §9 →
   e2e viewer sungguhan di terminal TTY (buka 2 terminal: serve + connect, gerakkan mouse,
   ketik, `Ctrl+Q` untuk keluar) — pastikan renderer & input bekerja nyata.
5. Opsional: `npm audit` masih melapor 2 moderate (kemungkinan dev deps) — nilai ulang.

## 9. Smoke test e2e terakhir (sudah lolos)

```powershell
npm run build
node dist/main.js serve --token test123 --address 127.0.0.1:59010 --fps 5 --log-level debug   # terminal 1
node dist/main.js info 127.0.0.1:59010 --token test123      # → screen: 1920x1080, fps: 5, server: 0.1.0, exit 0
node dist/main.js info 127.0.0.1:59010 --token wrong        # → "error: unauthorized", exit 3
```
Log server yang benar: `server listening` → `viewer connecting` → `viewer connected` →
`session closed {reason:"info"}` → `rejected unauthorized viewer`.

## 10. Catatan pitfalls

- Jangan pakai `process.stdout.write` selain dari renderer/logger di viewer (log = stderr).
- `serve()` menunggu `signal` abort — test harus `controller.abort()` + `await servePromise`
  atau test hang.
- `client.connect()` melempar error TTY dulu sebelum menyentuh jaringan (di `TerminalViewer.run`).
- Client busy membuang `Error{code}` jadi `GhostwireError('protocol', 'busy: ...')` (message berisi
  kode) — sesuaikan ekspektasi test.
- Frame identik dilewati server (`samePixels`) — FakeCapturer harus mengubah piksel tiap `grab()`.
- Keyboard shortcut viewer: **Ctrl+Q keluar** (Ctrl+Q TIDAK diteruskan ke remote); Ctrl+C remote
  diteruskan normal (raw mode mematikan ISIG).
- Info command sengaja tidak punya flag `--ping-*` (di-hardcode 5000/15000).
