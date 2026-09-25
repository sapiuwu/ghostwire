package main

import (
	"context"
	"os"
	"os/signal"

	"ghostwire/internal/adapter/capture"
	"ghostwire/internal/adapter/cli"
	"ghostwire/internal/adapter/compress"
	"ghostwire/internal/adapter/inject"
	"ghostwire/internal/adapter/logging"
	"ghostwire/internal/adapter/terminal"
	"ghostwire/internal/adapter/transport"
	"ghostwire/internal/port"
	"ghostwire/internal/service"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		<-sigCh
		cancel()
	}()

	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			compressor := compressorFor(opts.Compress)
			svc := service.NewServerService(service.ServerDeps{
				Transport:  &transport.TCPTransport{},
				Capturer:   capture.NewGdiCapturer(),
				Injector:   inject.NewSendInputInjector(),
				Compressor: compressor,
				Logger:     logging.NewConsoleLogger("info"),
			})
			return svc.Serve(opts, ctx)
		},
		Connect: func(opts port.ConnectOptions, ctx context.Context) error {
			svc := service.NewClientService(service.ClientDeps{
				Transport:  &transport.TCPTransport{},
				Compressor: compress.NewDeflateCompressor(1),
				Logger:     logging.NewConsoleLogger("info"),
			})
			viewer := terminal.NewTerminalViewer(terminal.TerminalViewerDeps{
				Viewer: svc,
				Logger: logging.NewConsoleLogger("info"),
			})
			return viewer.Run(ctx, opts)
		},
		FetchInfo: func(opts port.ConnectOptions) (port.SessionInfo, error) {
			svc := service.NewClientService(service.ClientDeps{
				Transport:  &transport.TCPTransport{},
				Compressor: compress.NewDeflateCompressor(1),
				Logger:     logging.NewConsoleLogger("info"),
			})
			session, err := svc.Connect(ctx, opts)
			if err != nil {
				return port.SessionInfo{}, err
			}
			defer session.Close("info")
			return session.Info(), nil
		},
	}

	code := cli.RunCli(ctx, cancel, handlers)
	os.Exit(code)
}

func compressorFor(mode string) port.CompressorPort {
	if mode == "none" {
		return &compress.NullCompressor{}
	}
	return compress.NewDeflateCompressor(1)
}


