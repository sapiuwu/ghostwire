package cli_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ghostwire/internal/adapter/cli"
	"ghostwire/internal/port"
)

func setupTokenEnv(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("GHOSTWIRE_TOKEN", "")
}

func defaultTokenFile(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	return filepath.Join(wd, "ghostwire.token")
}

func runCli(t *testing.T, handlers cli.CliHandlers, args ...string) (error, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := cli.CreateCli(handlers, ctx, cancel)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	return cmd.Execute(), &out, &errOut
}

func TestTokenGenerateCreatesFile(t *testing.T) {
	setupTokenEnv(t)

	err, out, _ := runCli(t, cli.CliHandlers{}, "token", "generate")
	if err != nil {
		t.Fatalf("token generate failed: %v", err)
	}
	token := strings.TrimSpace(out.String())
	if token == "" {
		t.Fatal("expected token printed to stdout")
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("expected 32 entropy bytes, got %d", len(raw))
	}
	data, err := os.ReadFile(defaultTokenFile(t))
	if err != nil {
		t.Fatalf("token file not written: %v", err)
	}
	if strings.TrimSpace(string(data)) != token {
		t.Errorf("token file content %q does not match stdout %q", strings.TrimSpace(string(data)), token)
	}
}

func TestTokenGenerateRefusesOverwrite(t *testing.T) {
	setupTokenEnv(t)

	if err, _, _ := runCli(t, cli.CliHandlers{}, "token", "generate"); err != nil {
		t.Fatalf("first generate failed: %v", err)
	}
	err, _, _ := runCli(t, cli.CliHandlers{}, "token", "generate")
	if err == nil {
		t.Fatal("expected error when token file already exists")
	}
	if err, _, _ := runCli(t, cli.CliHandlers{}, "token", "generate", "--force"); err != nil {
		t.Fatalf("generate --force failed: %v", err)
	}
}

func TestTokenGenerateCustomPath(t *testing.T) {
	setupTokenEnv(t)
	path := filepath.Join(t.TempDir(), "nested", "custom.token")

	err, out, _ := runCli(t, cli.CliHandlers{}, "token", "generate", "--path", path)
	if err != nil {
		t.Fatalf("token generate --path failed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("custom token file not written: %v", err)
	}
	if strings.TrimSpace(string(data)) != strings.TrimSpace(out.String()) {
		t.Error("custom token file content mismatch")
	}
	if _, err := os.Stat(defaultTokenFile(t)); !os.IsNotExist(err) {
		t.Error("default token file should not be created when --path is set")
	}
}

func TestServeUsesDefaultTokenFile(t *testing.T) {
	setupTokenEnv(t)
	path := defaultTokenFile(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("file-token-default\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var got string
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			got = opts.Token
			return nil
		},
	}
	if err, _, _ := runCli(t, handlers, "serve"); err != nil {
		t.Fatalf("serve failed: %v", err)
	}
	if got != "file-token-default" {
		t.Errorf("expected token from file, got %q", got)
	}
}

func TestServeTokenFileFlag(t *testing.T) {
	setupTokenEnv(t)
	path := filepath.Join(t.TempDir(), "custom.token")
	if err := os.WriteFile(path, []byte("flag-file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	var got string
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			got = opts.Token
			return nil
		},
	}
	if err, _, _ := runCli(t, handlers, "serve", "--token-file", path); err != nil {
		t.Fatalf("serve failed: %v", err)
	}
	if got != "flag-file-token" {
		t.Errorf("expected token from --token-file, got %q", got)
	}
}

func TestServeTokenPrecedence(t *testing.T) {
	setupTokenEnv(t)
	path := filepath.Join(t.TempDir(), "custom.token")
	if err := os.WriteFile(path, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	var got string
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			got = opts.Token
			return nil
		},
	}
	if err, _, _ := runCli(t, handlers, "serve", "--token", "flag-token", "--token-file", path); err != nil {
		t.Fatalf("serve with --token failed: %v", err)
	}
	if got != "flag-token" {
		t.Errorf("--token should win over token file, got %q", got)
	}

	got = ""
	if err, _, _ := runCli(t, handlers, "serve", "--token-file", path); err != nil {
		t.Fatalf("serve without --token failed: %v", err)
	}
	if got != "file-token" {
		t.Errorf("expected token file to win over env, got %q", got)
	}

	t.Setenv("GHOSTWIRE_TOKEN", "env-token")
	got = ""
	if err, _, _ := runCli(t, handlers, "serve", "--token-file", path); err != nil {
		t.Fatalf("serve with env token failed: %v", err)
	}
	if got != "env-token" {
		t.Errorf("GHOSTWIRE_TOKEN should win over token file, got %q", got)
	}
}

func TestServeTokenFileMissing(t *testing.T) {
	setupTokenEnv(t)

	var called bool
	handlers := cli.CliHandlers{
		Serve: func(opts port.ServeOptions, ctx context.Context) error {
			called = true
			return nil
		},
	}
	err, _, _ := runCli(t, handlers, "serve")
	if err == nil {
		t.Fatal("expected error when no token source is available")
	}
	if called {
		t.Error("serve handler must not run without a token")
	}
}

func TestServeExplicitTokenFileMissing(t *testing.T) {
	setupTokenEnv(t)
	path := filepath.Join(t.TempDir(), "does-not-exist.token")

	err, _, _ := runCli(t, cli.CliHandlers{}, "serve", "--token-file", path)
	if err == nil {
		t.Fatal("expected error for missing --token-file")
	}
}

func TestConnectUsesTokenFile(t *testing.T) {
	setupTokenEnv(t)
	path := filepath.Join(t.TempDir(), "custom.token")
	if err := os.WriteFile(path, []byte("connect-file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	var got string
	handlers := cli.CliHandlers{
		Connect: func(opts port.ConnectOptions, ctx context.Context) error {
			got = opts.Token
			return nil
		},
	}
	if err, _, _ := runCli(t, handlers, "connect", "127.0.0.1:5901", "--token-file", path); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	if got != "connect-file-token" {
		t.Errorf("expected token from --token-file, got %q", got)
	}
}
