package transport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// OpenSSH binds a temporary socket at ControlPath plus "." and 16 random
	// characters. macOS sockaddr_un.sun_path is 104 bytes.
	controlPathSuffix = 17
	maxControlPath    = 104
)

const controlPathTooLongNotice = "slurm-monitor: SSH multiplexing disabled because the ControlPath would exceed the 104-byte socket limit"

type muxState int

const (
	muxOn muxState = iota
	muxOffUnsafe
	muxOffTooLong
)

type SSHOptions struct {
	Target         string
	ConfigPath     string
	IdentityFile   string
	Port           int
	ConnectTimeout time.Duration
}

type SSHTransport struct {
	opts        SSHOptions
	controlPath string
	mux         muxState
}

func NewSSHTransport(opts SSHOptions) *SSHTransport {
	path, state := buildControlPath(opts)
	return &SSHTransport{
		opts:        opts,
		controlPath: path,
		mux:         state,
	}
}

func (t *SSHTransport) Describe() string {
	return "ssh:" + t.opts.Target
}

func (t *SSHTransport) Run(ctx context.Context, command string) (RunResult, error) {
	if notice := t.multiplexingNotice(); notice != "" {
		controlPathNotice.Do(func() {
			fmt.Fprintln(os.Stderr, notice)
		})
	}
	return run(ctx, t.Describe(), "ssh", t.buildSSHArgs(command)...)
}

var controlPathNotice sync.Once

func (t *SSHTransport) multiplexingNotice() string {
	if t.mux != muxOffTooLong {
		return ""
	}
	return controlPathTooLongNotice
}

func (t *SSHTransport) buildSSHArgs(command string) []string {
	args := make([]string, 0, 24)
	if t.opts.ConnectTimeout > 0 {
		seconds := max(int(math.Ceil(t.opts.ConnectTimeout.Seconds())), 1)
		args = append(args, "-o", fmt.Sprintf("ConnectTimeout=%d", seconds))
	}
	args = append(args,
		"-o", "ConnectionAttempts=2",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		"-o", "TCPKeepAlive=yes",
	)
	if t.controlPath != "" {
		args = append(args, "-o", "ControlMaster=auto", "-o", "ControlPersist=300", "-o", "StreamLocalBindUnlink=yes", "-o", "ControlPath="+t.controlPath)
	}

	if t.opts.ConfigPath != "" {
		args = append(args, "-F", t.opts.ConfigPath)
	}
	if t.opts.IdentityFile != "" {
		args = append(args, "-i", t.opts.IdentityFile)
	}
	if t.opts.Port > 0 {
		args = append(args, "-p", strconv.Itoa(t.opts.Port))
	}

	remoteCommand := "sh -lc " + shellQuote(command)
	args = append(args, "--", t.opts.Target, remoteCommand)
	return args
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

func buildControlPath(opts SSHOptions) (string, muxState) {
	base := fmt.Sprintf("%s|%s|%s|%d", opts.Target, opts.ConfigPath, opts.IdentityFile, opts.Port)
	sum := sha256.Sum256([]byte(base))
	id := hex.EncodeToString(sum[:8])
	root, err := controlDir()
	if err != nil {
		return "", muxOffUnsafe
	}
	path := filepath.Join(root, id)
	if len(path)+controlPathSuffix >= maxControlPath {
		return "", muxOffTooLong
	}
	return path, muxOn
}

func controlBase() string {
	if base := os.Getenv("XDG_RUNTIME_DIR"); base != "" {
		info, err := os.Stat(base)
		if err == nil && info.IsDir() {
			return base
		}
	}
	return os.TempDir()
}

func controlDir() (string, error) {
	dir := filepath.Join(controlBase(), fmt.Sprintf("slurm-mon-%d", os.Getuid()))
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || int(st.Uid) != os.Getuid() || info.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("unsafe SSH control directory %s", dir)
	}
	return dir, nil
}

func (t *SSHTransport) MultiplexingStatus() string {
	switch t.mux {
	case muxOffTooLong:
		return "disabled (ControlPath exceeds the 104-byte socket limit)"
	case muxOffUnsafe:
		return "disabled (no safe per-user control directory)"
	default:
		return filepath.Dir(t.controlPath)
	}
}
