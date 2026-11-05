package transport

import "context"

type LocalTransport struct{}

func NewLocalTransport() *LocalTransport   { return &LocalTransport{} }
func (t *LocalTransport) Describe() string { return "local" }
func (t *LocalTransport) Run(ctx context.Context, command string) (RunResult, error) {
	return run(ctx, t.Describe(), "sh", "-lc", command)
}
