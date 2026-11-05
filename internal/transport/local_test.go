package transport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTimeoutBoundsInheritedPipes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := NewLocalTransport().Run(ctx, "sleep 5; true")
	var runErr *RunError
	if !errors.As(err, &runErr) || !runErr.Timeout {
		t.Fatalf("expected timeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
}

func TestLocalTimeoutKillsProcessGroup(t *testing.T) {
	marker := fmt.Sprintf("sleep 47.%d", time.Now().UnixNano())
	t.Cleanup(func() { stopProcesses(marker) })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := NewLocalTransport().Run(ctx, marker+"; true")
	elapsed := time.Since(start)

	var runErr *RunError
	if !errors.As(err, &runErr) || !runErr.Timeout {
		t.Fatalf("expected timeout, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
	if survivors := processIDs(marker); len(survivors) > 0 {
		t.Fatalf("grandchild still running: %v", survivors)
	}
}

func TestSuccessfulCommandIgnoresPipeDelay(t *testing.T) {
	marker := fmt.Sprintf("sleep 48.%d", time.Now().UnixNano())
	t.Cleanup(func() { stopProcesses(marker) })

	start := time.Now()
	result, err := NewLocalTransport().Run(context.Background(), "echo ok; "+marker+" &")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !strings.Contains(result.Stdout, "ok") {
		t.Fatalf("stdout %q", result.Stdout)
	}
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("pipe cleanup took %v", elapsed)
	}
}

func processIDs(marker string) []int {
	out, err := exec.Command("ps", "-eo", "pid=,args=").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, marker) {
			continue
		}
		pidText, _, _ := strings.Cut(line, " ")
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid <= 0 || pid == os.Getpid() {
			continue
		}
		pids = append(pids, pid)
	}
	return pids
}

func stopProcesses(marker string) {
	for _, pid := range processIDs(marker) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
