package cli

import (
	"context"
	"fmt"
	"os"

	"ghostwire/internal/domain"
	"ghostwire/internal/port"
	"github.com/spf13/cobra"
)

type CliHandlers struct {
	Serve     func(opts port.ServeOptions, ctx context.Context) error
	Connect   func(opts port.ConnectOptions, ctx context.Context) error
	FetchInfo func(opts port.ConnectOptions) (port.SessionInfo, error)
}

const handshakeTimeoutMs = 10000

func CreateCli(handlers CliHandlers, ctx context.Context, cancel context.CancelFunc) *cobra.Command {
	root := &cobra.Command{
		Use:     "ghostwire",
		Short:   "remote desktop over a custom RDP-like protocol (GWRD)",
		Version: "0.1.0",
	}

	// serve command
	var serveAddr, serveToken, serveTokenFile string
	var serveFps, serveMaxWidth int
	var serveCompress string
	var serveTlsCert, serveTlsKey string
	var servePingInterval, servePingTimeout int
	var serveLogLevel string

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "share this machine screen over TCP",
		RunE: func(cmd *cobra.Command, args []string) error {
			token, tokenSource, err := resolveToken(serveToken, serveTokenFile)
			if err != nil {
				return err
			}
			if err := validateServe(token, serveFps, serveMaxWidth, servePingInterval, servePingTimeout, serveTlsCert, serveTlsKey); err != nil {
				return err
			}
			printTokenSource(cmd, tokenSource)
			opts := port.ServeOptions{
				Address:            serveAddr,
				Token:              token,
				Fps:                serveFps,
				MaxWidth:           serveMaxWidth,
				Compress:           serveCompress,
				HandshakeTimeoutMs: handshakeTimeoutMs,
				PingIntervalMs:     servePingInterval,
				PingTimeoutMs:      servePingTimeout,
			}
			if serveTlsCert != "" && serveTlsKey != "" {
				opts.Tls = &port.TlsListenOptions{CertPath: serveTlsCert, KeyPath: serveTlsKey}
			}
			return handlers.Serve(opts, ctx)
		},
	}
	serveCmd.Flags().StringVarP(&serveAddr, "address", "a", "0.0.0.0:5901", "listen address (host:port)")
	serveCmd.Flags().StringVarP(&serveToken, "token", "t", "", "auth token (env: GHOSTWIRE_TOKEN, fallback: token file)")
	serveCmd.Flags().StringVar(&serveTokenFile, "token-file", "", "auth token file (default ~/.ghostwire/token)")
	serveCmd.Flags().IntVar(&serveFps, "fps", 10, "capture frame rate (1-60)")
	serveCmd.Flags().IntVar(&serveMaxWidth, "max-width", 1600, "max frame width in pixels, 0 = native")
	serveCmd.Flags().StringVar(&serveCompress, "compress", "deflate", "frame compression (deflate|none)")
	serveCmd.Flags().StringVar(&serveTlsCert, "tls-cert", "", "TLS certificate path (PEM)")
	serveCmd.Flags().StringVar(&serveTlsKey, "tls-key", "", "TLS private key path (PEM)")
	serveCmd.Flags().IntVar(&servePingInterval, "ping-interval", 5000, "keepalive ping interval (ms)")
	serveCmd.Flags().IntVar(&servePingTimeout, "ping-timeout", 15000, "keepalive timeout (ms)")
	serveCmd.Flags().StringVar(&serveLogLevel, "log-level", "info", "log verbosity (debug|info|warn|error)")

	// connect command
	var connectAddr, connectToken, connectTokenFile, connectName string
	var connectTls bool
	var connectCaCert string
	var connectInsecure bool
	var connectPingInterval, connectPingTimeout int
	var connectLogLevel string

	connectCmd := &cobra.Command{
		Use:   "connect <host:port>",
		Short: "view and control a remote ghostwire server in this terminal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			connectAddr = args[0]
			token, tokenSource, err := resolveToken(connectToken, connectTokenFile)
			if err != nil {
				return err
			}
			if err := validateConnect(connectAddr, token, connectName, connectPingInterval, connectPingTimeout, connectTls, connectCaCert, connectInsecure); err != nil {
				return err
			}
			printTokenSource(cmd, tokenSource)
			opts := port.ConnectOptions{
				Address:            connectAddr,
				Token:              token,
				ClientName:         connectName,
				HandshakeTimeoutMs: handshakeTimeoutMs,
				PingIntervalMs:     connectPingInterval,
				PingTimeoutMs:      connectPingTimeout,
			}
			if connectTls {
				opts.Tls = &port.TlsDialOptions{Insecure: connectInsecure}
				if connectCaCert != "" {
					opts.Tls.CaCertPath = connectCaCert
				}
			}
			return handlers.Connect(opts, ctx)
		},
	}
	connectCmd.Flags().StringVarP(&connectToken, "token", "t", "", "auth token (env: GHOSTWIRE_TOKEN, fallback: token file)")
	connectCmd.Flags().StringVar(&connectTokenFile, "token-file", "", "auth token file (default ~/.ghostwire/token)")
	connectCmd.Flags().StringVarP(&connectName, "name", "n", "ghostwire-cli", "client name shown to the server")
	connectCmd.Flags().BoolVar(&connectTls, "tls", false, "use TLS for the connection")
	connectCmd.Flags().StringVar(&connectCaCert, "ca-cert", "", "custom CA certificate (PEM)")
	connectCmd.Flags().BoolVar(&connectInsecure, "insecure", false, "skip server certificate verification")
	connectCmd.Flags().IntVar(&connectPingInterval, "ping-interval", 5000, "keepalive ping interval (ms)")
	connectCmd.Flags().IntVar(&connectPingTimeout, "ping-timeout", 15000, "keepalive timeout (ms)")
	connectCmd.Flags().StringVar(&connectLogLevel, "log-level", "info", "log verbosity (debug|info|warn|error)")

	// info command
	var infoAddr, infoToken, infoTokenFile string
	var infoTls bool
	var infoCaCert string
	var infoInsecure bool
	var infoJson bool
	var infoLogLevel string

	infoCmd := &cobra.Command{
		Use:   "info <host:port>",
		Short: "query a remote server and print its session info",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			infoAddr = args[0]
			token, tokenSource, err := resolveToken(infoToken, infoTokenFile)
			if err != nil {
				return err
			}
			if err := validateConnect(infoAddr, token, "ghostwire-info", 5000, 15000, infoTls, infoCaCert, infoInsecure); err != nil {
				return err
			}
			printTokenSource(cmd, tokenSource)
			opts := port.ConnectOptions{
				Address:            infoAddr,
				Token:              token,
				ClientName:         "ghostwire-info",
				HandshakeTimeoutMs: handshakeTimeoutMs,
				PingIntervalMs:     5000,
				PingTimeoutMs:      15000,
			}
			if infoTls {
				opts.Tls = &port.TlsDialOptions{Insecure: infoInsecure}
				if infoCaCert != "" {
					opts.Tls.CaCertPath = infoCaCert
				}
			}
			info, err := handlers.FetchInfo(opts)
			if err != nil {
				return err
			}
			if infoJson {
				fmt.Printf("%s\n", fmt.Sprintf(`{"screen":{"width":%d,"height":%d},"fps":%d,"serverVersion":"%s"}`,
					info.Screen.Width, info.Screen.Height, info.Fps, info.ServerVersion))
				return nil
			}
			fmt.Printf("address: %s\nscreen:  %dx%d\nfps:     %d\nserver:  %s\n",
				infoAddr, info.Screen.Width, info.Screen.Height, info.Fps, info.ServerVersion)
			return nil
		},
	}
	infoCmd.Flags().StringVarP(&infoToken, "token", "t", "", "auth token (env: GHOSTWIRE_TOKEN, fallback: token file)")
	infoCmd.Flags().StringVar(&infoTokenFile, "token-file", "", "auth token file (default ~/.ghostwire/token)")
	infoCmd.Flags().BoolVar(&infoTls, "tls", false, "use TLS for the connection")
	infoCmd.Flags().StringVar(&infoCaCert, "ca-cert", "", "custom CA certificate (PEM)")
	infoCmd.Flags().BoolVar(&infoInsecure, "insecure", false, "skip server certificate verification")
	infoCmd.Flags().BoolVar(&infoJson, "json", false, "print raw JSON")
	infoCmd.Flags().StringVar(&infoLogLevel, "log-level", "info", "log verbosity (debug|info|warn|error)")

	// token command
	var tokenFile string
	var tokenForce bool

	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "manage the auth token file used by serve, connect and info",
	}
	tokenGenerateCmd := &cobra.Command{
		Use:   "generate",
		Short: "generate a new auth token and save it to the token file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolveTokenPath(tokenFile)
			if err != nil {
				return err
			}
			token, err := GenerateToken()
			if err != nil {
				return err
			}
			if err := SaveTokenFile(path, token, tokenForce); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			fmt.Fprintf(cmd.ErrOrStderr(), "token: saved to %s\n", path)
			return nil
		},
	}
	tokenGenerateCmd.Flags().StringVar(&tokenFile, "path", "", "token file path (default ~/.ghostwire/token)")
	tokenGenerateCmd.Flags().BoolVar(&tokenForce, "force", false, "overwrite an existing token file")
	tokenCmd.AddCommand(tokenGenerateCmd)

	root.AddCommand(serveCmd, connectCmd, infoCmd, tokenCmd)

	return root
}

func RunCli(ctx context.Context, cancel context.CancelFunc, handlers CliHandlers) int {
	cmd := CreateCli(handlers, ctx, cancel)
	if err := cmd.Execute(); err != nil {
		gw := domain.ToGhostwireError(err, domain.ErrInternal)
		fmt.Fprintf(os.Stderr, "error: %s\n", gw.Message)
		return exitCodeFor(gw.Code)
	}
	return 0
}

func validateServe(token string, fps, maxWidth, pingInterval, pingTimeout int, tlsCert, tlsKey string) error {
	if token == "" {
		return domain.NewError(domain.ErrConfig, "serve: token is required (--token, GHOSTWIRE_TOKEN, --token-file, or run 'ghostwire token generate')")
	}
	if err := requireInteger(fps, 1, 60, "serve: --fps"); err != nil {
		return err
	}
	if err := requireInteger(maxWidth, 0, 16384, "serve: --max-width"); err != nil {
		return err
	}
	if err := requireInteger(pingInterval, 100, 600000, "serve: --ping-interval"); err != nil {
		return err
	}
	if err := requireInteger(pingTimeout, 100, 600000, "serve: --ping-timeout"); err != nil {
		return err
	}
	if pingTimeout < pingInterval {
		return domain.NewError(domain.ErrConfig, "serve: --ping-timeout must be >= --ping-interval")
	}
	if (tlsCert != "") != (tlsKey != "") {
		return domain.NewError(domain.ErrConfig, "serve: --tls-cert and --tls-key must be used together")
	}
	return nil
}

func validateConnect(address, token, name string, pingInterval, pingTimeout int, tls bool, caCert string, insecure bool) error {
	if address == "" || len(address) == 0 {
		return domain.NewError(domain.ErrConfig, "connect: address must be host:port")
	}
	if token == "" {
		return domain.NewError(domain.ErrConfig, "connect: token is required (--token, GHOSTWIRE_TOKEN, or --token-file)")
	}
	if name == "" {
		return domain.NewError(domain.ErrConfig, "connect: --name must not be empty")
	}
	if err := requireInteger(pingInterval, 100, 600000, "connect: --ping-interval"); err != nil {
		return err
	}
	if err := requireInteger(pingTimeout, 100, 600000, "connect: --ping-timeout"); err != nil {
		return err
	}
	if pingTimeout < pingInterval {
		return domain.NewError(domain.ErrConfig, "connect: --ping-timeout must be >= --ping-interval")
	}
	if caCert != "" && !tls {
		return domain.NewError(domain.ErrConfig, "connect: --ca-cert requires --tls")
	}
	if insecure && !tls {
		return domain.NewError(domain.ErrConfig, "connect: --insecure requires --tls")
	}
	return nil
}

func requireInteger(value, min, max int, label string) error {
	if value < min || value > max {
		return domain.NewError(domain.ErrConfig,
			fmt.Sprintf("%s must be an integer between %d and %d", label, min, max))
	}
	return nil
}

func exitCodeFor(code domain.ErrorCode) int {
	switch code {
	case domain.ErrConfig:
		return 2
	case domain.ErrAuth:
		return 3
	case domain.ErrTimeout:
		return 4
	case domain.ErrBusy:
		return 5
	default:
		return 1
	}
}
