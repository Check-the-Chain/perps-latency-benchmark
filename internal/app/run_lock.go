package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"perps-latency-benchmark/internal/venues/spec"
)

type runLock struct {
	file        *os.File
	busyMessage string
}

func acquireRunLock(venueName string, cfg fileConfig) (*runLock, error) {
	target, ok := runLockTarget(venueName, cfg)
	if !ok {
		return &runLock{}, nil
	}
	path, err := runLockPath(target.Name)
	if err != nil || path == "" {
		return &runLock{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			if target.BusyMessage != "" {
				return nil, fmt.Errorf("%s", target.BusyMessage)
			}
			return nil, fmt.Errorf("run lock %s is already in use by another benchmark process", target.Name)
		}
		return nil, err
	}
	_ = file.Truncate(0)
	_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	return &runLock{file: file, busyMessage: target.BusyMessage}, nil
}

func (l *runLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}

func runLockTarget(venueName string, cfg fileConfig) (spec.RunLockTarget, bool) {
	runtime, ok := resolveVenueRuntime(venueName, cfg)
	if !ok {
		return spec.RunLockTarget{}, false
	}
	if runtime.Request.Builder.Type == "" {
		return spec.RunLockTarget{}, false
	}
	return runtime.Definition.RunLockTarget(runtime.RuntimeConfig())
}

func runLockPath(name string) (string, error) {
	lockDir := os.Getenv("PERPS_BENCH_LOCK_DIR")
	if lockDir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		lockDir = filepath.Join(cacheDir, "perps-latency-benchmark", "locks")
	}
	return filepath.Join(lockDir, lockPart(name)+".lock"), nil
}

func lockPart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
