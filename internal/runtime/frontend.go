package runtime

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func runFrontend(ctx context.Context, opts Options) error {
	mode := strings.ToLower(strings.TrimSpace(opts.FrontendMode))
	if mode == "" || mode == "off" || mode == "false" || mode == "0" {
		return nil
	}
	if mode != "dev" && mode != "build" {
		return fmt.Errorf("unsupported frontend mode %q, expected dev, build or off", opts.FrontendMode)
	}

	frontendDir := strings.TrimSpace(opts.FrontendDir)
	if frontendDir == "" {
		frontendDir = "frontend"
	}
	absDir, err := resolveFrontendDir(frontendDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(absDir); err != nil {
		return fmt.Errorf("frontend directory %s is not available: %w", absDir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("frontend path %s is not a directory", absDir)
	}

	packageManager := strings.TrimSpace(opts.FrontendPackageManager)
	if packageManager == "" {
		packageManager = "npm"
	}

	if mode == "build" {
		return runFrontendCommand(ctx, absDir, packageManager, []string{"run", "build"}, opts)
	}

	host := strings.TrimSpace(opts.FrontendHost)
	if host == "" {
		host = "0.0.0.0"
	}
	port := strings.TrimSpace(opts.FrontendPort)
	if port == "" {
		port = "3001"
	}
	return runFrontendCommand(ctx, absDir, packageManager, []string{"run", "dev", "--", "--host", host, "--port", port}, opts)
}

func resolveFrontendDir(frontendDir string) (string, error) {
	if filepath.IsAbs(frontendDir) {
		return frontendDir, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, frontendDir)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, frontendDir), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Abs(frontendDir)
		}
		dir = parent
	}
}

func runFrontendCommand(ctx context.Context, dir string, bin string, args []string, opts Options) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s is required to run frontend %s; install Node.js/npm first: %w", bin, opts.FrontendMode, err)
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = frontendEnv(opts)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	slog.Info("frontend command starting", "mode", opts.FrontendMode, "dir", dir, "command", bin+" "+strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go logFrontendPipe(&wg, stdout, false)
	go logFrontendPipe(&wg, stderr, true)

	err = cmd.Wait()
	wg.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return nil
}

func frontendEnv(opts Options) []string {
	env := os.Environ()
	env = setDefaultEnv(env, "BROWSER", "none")
	env = setDefaultEnv(env, "VITE_EXCHANGE_API_URL", "/api")
	env = setDefaultEnv(env, "VITE_EXCHANGE_PROXY_TARGET", envOrDefault("FRONTEND_API_TARGET", "http://127.0.0.1:8080"))
	if strings.TrimSpace(opts.FrontendPort) != "" {
		env = setDefaultEnv(env, "FRONTEND_PORT", strings.TrimSpace(opts.FrontendPort))
	}
	return env
}

func logFrontendPipe(wg *sync.WaitGroup, reader any, isError bool) {
	defer wg.Done()
	var scanner *bufio.Scanner
	switch value := reader.(type) {
	case *os.File:
		scanner = bufio.NewScanner(value)
	default:
		if readable, ok := reader.(interface{ Read([]byte) (int, error) }); ok {
			scanner = bufio.NewScanner(readable)
		}
	}
	if scanner == nil {
		return
	}
	for scanner.Scan() {
		if isError {
			slog.Warn("frontend", "line", scanner.Text())
		} else {
			slog.Info("frontend", "line", scanner.Text())
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, os.ErrClosed) {
		slog.Warn("frontend log stream closed", "error", err)
	}
}

func frontendEnabled(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	return mode != "" && mode != "off" && mode != "false" && mode != "0"
}

func envOrDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func setDefaultEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			return env
		}
	}
	return append(env, prefix+value)
}
