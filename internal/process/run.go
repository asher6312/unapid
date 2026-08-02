package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"
)

type Result struct {
	Stdout string
	Stderr string
	Code   int
}

type Runner interface {
	Capture(context.Context, string, ...string) (Result, error)
	Interactive(context.Context, []string, string, ...string) error
}

type Local struct{}

func (Local) Capture(ctx context.Context, name string, args ...string) (Result, error) {
	if name == "" {
		return Result{}, errors.New("an executable is required")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String(), Code: 0}
	if err == nil {
		return result, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.Code = exitError.ExitCode()
		return result, nil
	}
	return Result{}, fmt.Errorf("start %s: %w", name, err)
}

func (Local) Interactive(ctx context.Context, env []string, name string, args ...string) error {
	if name == "" {
		return errors.New("an executable is required")
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", name, err)
	}
	return nil
}

func Detail(result Result) string {
	raw := result.Stderr
	if strings.TrimSpace(raw) == "" {
		raw = result.Stdout
	}
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, raw)
	clean = strings.TrimSpace(clean)
	if len(clean) > 800 {
		clean = clean[len(clean)-800:]
	}
	return clean
}
