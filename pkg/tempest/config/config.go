// Package config parses the small environment configuration shared by the
// TempestKeep commands.
package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	// DefaultDB is the working-directory archive name used when it exists.
	DefaultDB = "tempest.sqlite"
	// MaxDotenvBytes bounds configuration file reads and direct parsing.
	MaxDotenvBytes     = 1 << 20
	maxDotenvLineBytes = 64 << 10
	maxDotenvVariables = 256
	maxEnvKeyBytes     = 256
)

var ErrInvalidConfig = errors.New("invalid configuration")

// ErrConfigIO reports a configuration file or environment I/O failure. The
// returned error does not contain a path, key, or value.
var ErrConfigIO = errors.New("configuration I/O failure")

// APISettings holds validated ambient settings for the API client. An empty
// BaseURL means the client's default endpoint. CacheTTL is always set.
type APISettings struct {
	BaseURL  string
	CacheTTL time.Duration
}

// LoadDotenv loads a regular KEY=VALUE file without replacing variables that
// are already set. A missing file is not an error. The file is limited to
// MaxDotenvBytes and the context is checked before and after the read.
func LoadDotenv(ctx context.Context, path string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	pathInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return configIO("inspect dotenv", err)
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() {
		return fmt.Errorf("%w: dotenv must be a regular file, not a symlink", ErrInvalidConfig)
	}
	if pathInfo.Size() > MaxDotenvBytes {
		return fmt.Errorf("%w: dotenv exceeds %d bytes", ErrInvalidConfig, MaxDotenvBytes)
	}
	if runtime.GOOS != "windows" && pathInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: dotenv permissions must not grant group or other access", ErrInvalidConfig)
	}
	f, err := os.Open(path)
	if err != nil {
		return configIO("open dotenv", err)
	}
	defer func() { err = errors.Join(err, configIO("close dotenv", f.Close())) }()
	info, err := f.Stat()
	if err != nil {
		return configIO("inspect open dotenv", err)
	}
	if !os.SameFile(pathInfo, info) {
		return fmt.Errorf("%w: dotenv changed while it was opened", ErrInvalidConfig)
	}
	if info.Size() > MaxDotenvBytes {
		return fmt.Errorf("%w: dotenv exceeds %d bytes", ErrInvalidConfig, MaxDotenvBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxDotenvBytes+1))
	if err != nil {
		return configIO("read dotenv", err)
	}
	if len(data) > MaxDotenvBytes {
		return fmt.Errorf("%w: dotenv exceeds %d bytes", ErrInvalidConfig, MaxDotenvBytes)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	values, err := ParseDotenv(data)
	if err != nil {
		return err
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return configIO("set variable from dotenv", err)
		}
	}
	return nil
}

// ParseDotenv parses the KEY=VALUE subset written by FormatDotenvValue.
// Blank lines and comments are ignored. An optional "export " prefix is
// accepted. Malformed lines, duplicate keys, invalid quoting, NUL bytes, and
// configured size limits return ErrInvalidConfig.
func ParseDotenv(data []byte) (map[string]string, error) {
	if len(data) > MaxDotenvBytes {
		return nil, fmt.Errorf("%w: dotenv exceeds %d bytes", ErrInvalidConfig, MaxDotenvBytes)
	}
	out := make(map[string]string)
	lineNumber := 0
	for line := range strings.SplitSeq(string(data), "\n") {
		lineNumber++
		if len(line) > maxDotenvLineBytes {
			return nil, fmt.Errorf("%w: dotenv line %d exceeds %d bytes", ErrInvalidConfig, lineNumber, maxDotenvLineBytes)
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, raw, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !validEnvKey(key) {
			return nil, fmt.Errorf("%w: invalid dotenv assignment on line %d", ErrInvalidConfig, lineNumber)
		}
		if len(key) > maxEnvKeyBytes {
			return nil, fmt.Errorf("%w: dotenv key on line %d exceeds %d bytes", ErrInvalidConfig, lineNumber, maxEnvKeyBytes)
		}
		if _, duplicate := out[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate dotenv key on line %d", ErrInvalidConfig, lineNumber)
		}
		value, err := parseDotenvValue(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %w", ErrInvalidConfig, lineNumber, err)
		}
		if strings.IndexByte(value, 0) >= 0 {
			return nil, fmt.Errorf("%w: NUL in dotenv value on line %d", ErrInvalidConfig, lineNumber)
		}
		out[key] = value
		if len(out) > maxDotenvVariables {
			return nil, fmt.Errorf("%w: dotenv exceeds %d variables", ErrInvalidConfig, maxDotenvVariables)
		}
	}
	return out, nil
}

func validEnvKey(key string) bool {
	if key == "" || !isEnvKeyStart(key[0]) {
		return false
	}
	for i := 1; i < len(key); i++ {
		if !isEnvKeyStart(key[i]) && (key[i] < '0' || key[i] > '9') {
			return false
		}
	}
	return true
}

func isEnvKeyStart(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z'
}

func parseDotenvValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	switch value[0] {
	case '"':
		if len(value) < 2 || value[len(value)-1] != '"' {
			return "", errors.New("unterminated double-quoted value")
		}
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", errors.New("invalid double-quoted value")
		}
		return unquoted, nil
	case '\'':
		if len(value) < 2 || value[len(value)-1] != '\'' {
			return "", errors.New("unterminated single-quoted value")
		}
		return value[1 : len(value)-1], nil
	default:
		if strings.ContainsAny(value, "\"'") {
			return "", errors.New("quote in unquoted value")
		}
		return value, nil
	}
}

// FormatDotenvValue returns a value that ParseDotenv reads exactly.
func FormatDotenvValue(value string) (string, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return "", fmt.Errorf("%w: dotenv values cannot contain NUL", ErrInvalidConfig)
	}
	if len(value) > maxDotenvLineBytes {
		return "", fmt.Errorf("%w: dotenv value exceeds %d bytes", ErrInvalidConfig, maxDotenvLineBytes)
	}
	if value == "" || strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.ContainsAny(value, "#\"'\\") {
		return strconv.Quote(value), nil
	}
	return value, nil
}

// FirstNonEmpty returns the first non-empty string.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// ResolveDB returns the explicit path, TEMPEST_DB, or DefaultDB when that file
// exists. It performs no creation or migration.
func ResolveDB(ctx context.Context, explicit string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if path := FirstNonEmpty(explicit, os.Getenv("TEMPEST_DB")); path != "" {
		return path, nil
	}
	_, err := os.Stat(DefaultDB)
	switch {
	case err == nil:
		return DefaultDB, nil
	case errors.Is(err, os.ErrNotExist):
		return "", nil
	default:
		return "", configIO("inspect default archive", err)
	}
}

func configIO(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrConfigIO, action)
}

// APISettingsFromEnv parses TEMPEST_API_BASE and TEMPEST_CACHE_TTL. Cache TTL
// is seconds in the inclusive range 0..86400. Invalid values fail closed.
func APISettingsFromEnv() (APISettings, error) {
	settings := APISettings{BaseURL: os.Getenv("TEMPEST_API_BASE"), CacheTTL: 5 * time.Minute}
	if raw := os.Getenv("TEMPEST_CACHE_TTL"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 0 || seconds > 24*60*60 {
			return APISettings{}, fmt.Errorf("%w: TEMPEST_CACHE_TTL must be an integer from 0 to 86400 seconds", ErrInvalidConfig)
		}
		settings.CacheTTL = time.Duration(seconds) * time.Second
	}
	return settings, nil
}
