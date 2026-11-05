package transport

import (
	"context"
	"errors"
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
