package payload

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"perps-latency-benchmark/internal/secrets"
)

type CommandConfig struct {
	Command   []string
	Env       map[string]string
	Timeout   time.Duration
	Directory string
}

type CommandBuilder struct {
	cfg CommandConfig
}

func NewCommandBuilder(cfg CommandConfig) (*CommandBuilder, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("builder command is required")
	}
	return &CommandBuilder{cfg: cfg}, nil
}

func (b *CommandBuilder) Build(ctx context.Context, req Request) (Built, error) {
	if b.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.cfg.Timeout)
		defer cancel()
	}

	stdin, err := json.Marshal(req)
	if err != nil {
		return Built{}, err
	}

	cmd := commandContext(ctx, b.cfg)
	cmd.Stdin = bytes.NewReader(stdin)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Built{}, fmt.Errorf("builder command failed: %w: %s", err, secrets.RedactString(stderr.String()))
	}

	var built Built
	if err := json.Unmarshal(stdout.Bytes(), &built); err != nil {
		return Built{}, fmt.Errorf("decode builder response: %w: %s", err, secrets.RedactString(stdout.String()))
	}
	return built, nil
}

type PersistentCommandBuilder struct {
	cfg CommandConfig

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr safeBuffer
}

func NewPersistentCommandBuilder(cfg CommandConfig) (*PersistentCommandBuilder, error) {
	if len(cfg.Command) == 0 {
		return nil, fmt.Errorf("builder command is required")
	}
	return &PersistentCommandBuilder{cfg: cfg}, nil
}

func (b *PersistentCommandBuilder) Build(ctx context.Context, req Request) (Built, error) {
	if b.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.cfg.Timeout)
		defer cancel()
	}

	stdin, err := json.Marshal(req)
	if err != nil {
		return Built{}, err
	}
	stdin = append(stdin, '\n')

	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.ensureStartedLocked(); err != nil {
		return Built{}, err
	}
	if _, err := b.stdin.Write(stdin); err != nil {
		b.closeLocked()
		return Built{}, fmt.Errorf("builder command write failed: %w: %s", err, secrets.RedactString(b.stderr.String()))
	}

	line, err := b.readLineLocked(ctx)
	if err != nil {
		b.closeLocked()
		return Built{}, err
	}

	var built Built
	if err := json.Unmarshal(line, &built); err != nil {
		return Built{}, fmt.Errorf("decode builder response: %w: %s", err, secrets.RedactString(string(line)))
	}
	return built, nil
}

func (b *PersistentCommandBuilder) Close(context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closeLocked()
}

func (b *PersistentCommandBuilder) ensureStartedLocked() error {
	if b.cmd != nil && b.cmd.Process != nil {
		return nil
	}
	cmd := commandContext(context.Background(), b.cfg)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = &b.stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start builder command: %w", err)
	}
	b.cmd = cmd
	b.stdin = stdin
	b.stdout = bufio.NewReader(stdout)
	go func() { _ = cmd.Wait() }()
	return nil
}

func (b *PersistentCommandBuilder) readLineLocked(ctx context.Context) ([]byte, error) {
	type readResult struct {
		line []byte
		err  error
	}
	done := make(chan readResult, 1)
	go func() {
		line, err := b.stdout.ReadBytes('\n')
		done <- readResult{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		_ = b.closeLocked()
		return nil, fmt.Errorf("builder command timed out: %w: %s", ctx.Err(), secrets.RedactString(b.stderr.String()))
	case result := <-done:
		if result.err != nil {
			return nil, fmt.Errorf("builder command read failed: %w: %s", result.err, secrets.RedactString(b.stderr.String()))
		}
		return bytes.TrimSpace(result.line), nil
	}
}

func (b *PersistentCommandBuilder) closeLocked() error {
	var err error
	if b.stdin != nil {
		err = b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		if killErr := killProcessGroup(b.cmd); killErr != nil && err == nil {
			err = killErr
		}
	}
	b.cmd = nil
	b.stdin = nil
	b.stdout = nil
	return err
}

func commandContext(ctx context.Context, cfg CommandConfig) *exec.Cmd {
	cmd := exec.CommandContext(ctx, cfg.Command[0], cfg.Command[1:]...)
	cmd.Dir = cfg.Directory
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return killProcessGroup(cmd)
	}
	cmd.WaitDelay = 100 * time.Millisecond
	return cmd
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		if err == syscall.ESRCH {
			return nil
		}
		return err
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
