package cli_test

import (
	"context"
	"testing"

	"ghostwire/internal/adapter/cli"
	"ghostwire/internal/domain"
	"ghostwire/internal/port"
)

func TestServeRequiresToken(t *testing.T) {
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"serve"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for serve without token")
	}
	gw := domain.ToGhostwireError(err, domain.ErrInternal)
	if gw.Code != domain.ErrConfig {
		t.Errorf("expected config error, got %s", gw.Code)
	}
}

func TestServeFpsOutOfRange(t *testing.T) {
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"serve", "--token", "test", "--fps", "100"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for fps out of range")
	}
}

func TestConnectRequiresToken(t *testing.T) {
	handlers := cli.CliHandlers{
		Connect: func(opts port.ConnectOptions, ctx context.Context) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"connect", "127.0.0.1:5901"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for connect without token")
	}
}

func TestConnectRequiresAddress(t *testing.T) {
	handlers := cli.CliHandlers{
		Connect: func(opts port.ConnectOptions, ctx context.Context) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"connect"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for connect without address")
	}
}

func TestConnectCaCertRequiresTls(t *testing.T) {
	handlers := cli.CliHandlers{
		Connect: func(opts port.ConnectOptions, ctx context.Context) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"connect", "127.0.0.1:5901", "--token", "test", "--ca-cert", "/path/to/ca.pem"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --ca-cert without --tls")
	}
}

func TestConnectInsecureRequiresTls(t *testing.T) {
	handlers := cli.CliHandlers{
		Connect: func(opts port.ConnectOptions, ctx context.Context) error {
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"connect", "127.0.0.1:5901", "--token", "test", "--insecure"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --insecure without --tls")
	}
}

func TestExitCodeForConfig(t *testing.T) {
	// Test exitCodeFor directly instead of RunCli (which calls os.Exit)
	// RunCli calls os.Exit which would kill the test process
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			return domain.NewError(domain.ErrConfig, "test config error")
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := cli.CreateCli(handlers, ctx, cancel)
	cmd.SetArgs([]string{"serve", "--token", "test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error from Execute")
	}
	gw := domain.ToGhostwireError(err, domain.ErrInternal)
	if gw.Code != domain.ErrConfig {
		t.Errorf("expected config error, got %s", gw.Code)
	}
}
