package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"

	"perps-latency-benchmark/internal/venues/registry"
)

type runLock struct {
	file *os.File
}

func acquireRunLock(venueName string, cfg fileConfig) (*runLock, error) {
	path, err := runLockPath(venueName, cfg)
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
			return nil, fmt.Errorf("lighter account/API key is already in use by another benchmark process; use one Lighter API key per concurrent runner or stop the existing process")
		}
		return nil, err
	}
	_ = file.Truncate(0)
	_, _ = fmt.Fprintf(file, "pid=%d\n", os.Getpid())
	return &runLock{file: file}, nil
}

func (l *runLock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	_ = l.file.Close()
	l.file = nil
}

func runLockPath(venueName string, cfg fileConfig) (string, error) {
	if venueName != "lighter" {
		return "", nil
	}
	definition, ok := registry.Lookup(venueName)
	if !ok {
		return "", nil
	}
	request := mergeRequest(cfg.Request, cfgForVenue(definition.Name, cfg).Request)
	if request.Builder.Type == "" {
		return "", nil
	}
	params := definition.BuilderParams.Merge(request.Builder.Params)
	accountIndex := runtimeParam(params, "account_index", "LIGHTER_ACCOUNT_INDEX")
	apiKeyIndex := runtimeParam(params, "api_key_index", "LIGHTER_API_KEY_INDEX")
	if accountIndex == "" || apiKeyIndex == "" {
		return "", nil
	}
	lockDir := os.Getenv("PERPS_BENCH_LOCK_DIR")
	if lockDir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		lockDir = filepath.Join(cacheDir, "perps-latency-benchmark", "locks")
	}
	name := fmt.Sprintf("lighter-account-%s-api-key-%s.lock", lockPart(accountIndex), lockPart(apiKeyIndex))
	return filepath.Join(lockDir, name), nil
}

func runtimeParam(params map[string]any, key string, envKey string) string {
	if value, ok := params[key]; ok && value != nil {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" {
			return text
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
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
