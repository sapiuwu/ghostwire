package cli

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ghostwire/internal/domain"
	"github.com/spf13/cobra"
)

const tokenEntropyBytes = 32

func DefaultTokenPath() string {
	return "ghostwire.token"
}

func resolveTokenPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return DefaultTokenPath()
}

func GenerateToken() (string, error) {
	buf := make([]byte, tokenEntropyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.WrapError(err, domain.ErrInternal, "token: random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func SaveTokenFile(path, token string, force bool) error {
	if !force {
		_, err := os.Stat(path)
		if err == nil {
			return domain.NewError(domain.ErrConfig, "token: file already exists: "+path+" (use --force to overwrite)")
		}
		if !os.IsNotExist(err) {
			return domain.WrapError(err, domain.ErrConfig, "token: cannot stat "+path)
		}
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return domain.WrapError(err, domain.ErrConfig, "token: cannot create directory "+dir)
		}
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return domain.WrapError(err, domain.ErrConfig, "token: cannot write "+path)
	}
	return nil
}

func LoadTokenFile(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", domain.WrapError(err, domain.ErrConfig, "token: cannot read "+path)
	}
	return strings.TrimSpace(string(data)), nil
}

func resolveToken(explicit, tokenFile string) (string, string, error) {
	if explicit != "" {
		return explicit, "", nil
	}
	if env := os.Getenv("GHOSTWIRE_TOKEN"); env != "" {
		return env, "", nil
	}
	path := resolveTokenPath(tokenFile)
	token, err := LoadTokenFile(path)
	if err != nil {
		return "", "", err
	}
	if token == "" {
		if tokenFile != "" {
			return "", "", domain.NewError(domain.ErrConfig, "token file not found or empty: "+path)
		}
		return "", "", nil
	}
	return token, path, nil
}

func printTokenSource(cmd *cobra.Command, source string) {
	if source != "" {
		fmt.Fprintf(cmd.ErrOrStderr(), "%s: using token from %s\n", cmd.Name(), source)
	}
}
