package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Executor interface {
	Execute(ctx context.Context, command string, workDir string) (string, error)
}

type DirectExecutor struct {
	timeoutSec     int
	maxOutputBytes int
}

func NewDirectExecutor(timeoutSec, maxOutputBytes int) *DirectExecutor {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	if maxOutputBytes <= 0 {
		maxOutputBytes = 100_000
	}
	return &DirectExecutor{
		timeoutSec:     timeoutSec,
		maxOutputBytes: maxOutputBytes,
	}
}

func (e *DirectExecutor) Execute(ctx context.Context, command string, workDir string) (string, error) {
	timeout := time.Duration(e.timeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if len(output) > e.maxOutputBytes {
		output = output[:e.maxOutputBytes] + fmt.Sprintf("\n... (truncated at %d bytes)", e.maxOutputBytes)
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("command timed out after %ds", e.timeoutSec)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %w", err)
	}
	return output, nil
}
