// Command flui-server runs the flui web UI, API and storage.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ahmettsezis/flui/internal/auth"
	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/pki"
	"github.com/ahmettsezis/flui/internal/server"
	"github.com/ahmettsezis/flui/internal/store"
	"github.com/ahmettsezis/flui/internal/version"
)

const usage = `Usage: flui-server [command] [--config path]

Commands:
  serve             Run the server (default)
  tls fingerprint   Print the SHA-256 fingerprints of the server certificate and internal CA
  user reset-password <username>
                    Set a new random password (must be changed at next sign-in) and unlock the account
  version           Print version information

Options:
  --config path     Configuration file (default /etc/flui/flui.yaml)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = cmdServe(args, stderr)
	case "tls":
		err = cmdTLS(args, stdout)
	case "user":
		err = cmdUser(args, stdout)
	case "version":
		_, err = fmt.Fprintln(stdout, "flui-server", version.String())
	case "help":
		_, err = fmt.Fprint(stdout, usage)
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	var uerr usageError
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.As(err, &uerr):
		fmt.Fprintf(stderr, "%v\n\n%s", err, usage)
		return 2
	default:
		fmt.Fprintln(stderr, "error:", err)
		return 1
	}
}

type usageError string

func (e usageError) Error() string { return string(e) }

// parse parses common flags and loads the configuration. The config file is required only if --config was given.
func parse(name string, args []string) (config.Config, []string, error) {
	fset := flag.NewFlagSet(name, flag.ContinueOnError)
	fset.SetOutput(io.Discard)
	path := fset.String("config", config.DefaultPath, "configuration file")
	if err := fset.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return config.Config{}, nil, err
		}
		return config.Config{}, nil, usageError(err.Error())
	}
	explicit := false
	fset.Visit(func(f *flag.Flag) { explicit = explicit || f.Name == "config" })
	cfg, err := config.Load(*path, explicit, os.Environ())
	if err != nil {
		return cfg, nil, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return cfg, fset.Args(), nil
}

func cmdServe(args []string, stderr io.Writer) error {
	cfg, rest, err := parse("serve", args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return usageError("serve takes no arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return server.Run(ctx, cfg, server.NewLogger(cfg.Log, stderr), nil)
}

func cmdUser(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "reset-password" {
		return usageError("usage: flui-server user reset-password <username>")
	}
	cfg, rest, err := parse("user", args[1:])
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usageError("usage: flui-server user reset-password <username>")
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, store.FileName)); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no database found in %s", cfg.DataDir)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.Migrate(ctx); err != nil {
		return err
	}
	svc, err := auth.NewService(st.DB, cfg.Auth)
	if err != nil {
		return err
	}
	pw, err := svc.ResetPassword(ctx, rest[0])
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "New password for %s: %s\nIt must be changed at the next sign-in.\n", rest[0], pw)
	return err
}

func cmdTLS(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "fingerprint" {
		return usageError("usage: flui-server tls fingerprint")
	}
	cfg, _, err := parse("tls", args[1:])
	if err != nil {
		return err
	}
	serverCert := cfg.Server.TLS.CertFile
	if serverCert == "" {
		serverCert = filepath.Join(server.TLSDir(cfg), pki.ServerCertFile)
	}
	for _, c := range []struct{ label, path string }{
		{"Server certificate", serverCert},
		{"Internal CA", filepath.Join(server.TLSDir(cfg), pki.CACertFile)},
	} {
		fp, err := pki.FileFingerprint(c.path)
		if err != nil {
			return fmt.Errorf("%s: %w (has flui-server been started?)", c.label, err)
		}
		if _, err := fmt.Fprintf(stdout, "%-22s %s\n", c.label+":", fp); err != nil {
			return err
		}
	}
	return nil
}
