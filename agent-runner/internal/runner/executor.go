package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

var (
	ErrCommandPolicy = errors.New("command violates runner policy")
	ErrOutputLimit   = errors.New("command output limit exceeded")
)

const (
	maximumCommandOutput   = 64 * 1024
	commandTerminationWait = time.Second
)

type CommandResult struct {
	Arguments []string
	ExitCode  int
	Stdout    string
	Stderr    string
}

type Executor struct {
	Workspace          string
	AllowedExecutables []string
	OutputLimit        int
}

func DefaultExecutor(workspace string) Executor {
	return Executor{Workspace: workspace, AllowedExecutables: []string{defaultPythonExecutable()}, OutputLimit: maximumCommandOutput}
}

func defaultPythonExecutable() string {
	const imagePython = "/usr/local/bin/python3"
	if _, err := os.Stat(imagePython); err == nil {
		return imagePython
	}
	path, err := exec.LookPath("python3")
	if err != nil {
		return imagePython
	}
	return path
}

func (executor Executor) Run(ctx context.Context, arguments []string, workingDirectory string, environment map[string]string) (CommandResult, error) {
	if len(arguments) == 0 || containsUnsafeArgument(arguments) {
		return CommandResult{}, fmt.Errorf("%w: command arguments are invalid", ErrCommandPolicy)
	}
	if !slices.Contains(executor.AllowedExecutables, arguments[0]) {
		return CommandResult{}, fmt.Errorf("%w: executable is not allowed", ErrCommandPolicy)
	}
	workspace, err := filepath.EvalSymlinks(executor.Workspace)
	if err != nil {
		return CommandResult{}, fmt.Errorf("resolve workspace: %w", err)
	}
	directory, err := confinedDirectory(workspace, workingDirectory)
	if err != nil {
		return CommandResult{}, err
	}
	if err := validateEnvironment(environment); err != nil {
		return CommandResult{}, err
	}
	limit := executor.OutputLimit
	if limit == 0 {
		limit = maximumCommandOutput
	}
	stdout, stderr := &boundedBuffer{limit: limit}, &boundedBuffer{limit: limit}
	command := exec.Command(arguments[0], arguments[1:]...)
	command.Dir = directory
	command.Env = minimalEnvironment(environment)
	command.Stdout, command.Stderr = stdout, stderr
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return CommandResult{}, fmt.Errorf("start command: %w", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-wait:
	case <-ctx.Done():
		terminateProcessGroup(command.Process)
		select {
		case waitErr = <-wait:
		case <-time.After(commandTerminationWait):
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			waitErr = <-wait
		}
		return commandResult(arguments, stdout, stderr, waitErr), ctx.Err()
	}
	result := commandResult(arguments, stdout, stderr, waitErr)
	if stdout.exceeded || stderr.exceeded {
		return result, ErrOutputLimit
	}
	if waitErr != nil {
		return result, fmt.Errorf("command exited non-zero: %w", waitErr)
	}
	return result, nil
}

func containsUnsafeArgument(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "" || strings.ContainsRune(argument, '\x00') {
			return true
		}
	}
	return false
}

func confinedDirectory(workspace, requested string) (string, error) {
	if requested == "" {
		requested = "."
	}
	if filepath.IsAbs(requested) {
		return "", fmt.Errorf("%w: working directory must be relative", ErrCommandPolicy)
	}
	candidate := filepath.Join(workspace, requested)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("%w: resolve working directory", ErrCommandPolicy)
	}
	relative, err := filepath.Rel(workspace, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: working directory escapes workspace", ErrCommandPolicy)
	}
	return resolved, nil
}

func validateEnvironment(environment map[string]string) error {
	for key, value := range environment {
		if (key != "PYTHONHASHSEED" && key != "PYTHONDONTWRITEBYTECODE") || len(value) > 128 || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("%w: environment value is not allowed", ErrCommandPolicy)
		}
	}
	return nil
}

func minimalEnvironment(environment map[string]string) []string {
	values := []string{"HOME=/home/agent", "PATH=/usr/local/bin:/usr/bin:/bin", "TMPDIR=/tmp", "LANG=C.UTF-8"}
	keys := make([]string, 0, len(environment))
	for key := range environment {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		values = append(values, key+"="+environment[key])
	}
	return values
}

func terminateProcessGroup(process *os.Process) {
	if process != nil {
		_ = syscall.Kill(-process.Pid, syscall.SIGTERM)
	}
}

func commandResult(arguments []string, stdout, stderr *boundedBuffer, waitErr error) CommandResult {
	result := CommandResult{Arguments: slices.Clone(arguments), ExitCode: 0, Stdout: stdout.String(), Stderr: stderr.String()}
	var exitError *exec.ExitError
	if errors.As(waitErr, &exitError) {
		result.ExitCode = exitError.ExitCode()
	} else if waitErr != nil {
		result.ExitCode = -1
	}
	return result
}

type boundedBuffer struct {
	bytes    []byte
	limit    int
	exceeded bool
}

func (buffer *boundedBuffer) Write(value []byte) (int, error) {
	remaining := buffer.limit - len(buffer.bytes)
	if remaining > 0 {
		if len(value) > remaining {
			buffer.bytes = append(buffer.bytes, value[:remaining]...)
			buffer.exceeded = true
			return len(value), ErrOutputLimit
		}
		buffer.bytes = append(buffer.bytes, value...)
	}
	if len(value) > remaining {
		buffer.exceeded = true
		return len(value), ErrOutputLimit
	}
	return len(value), nil
}
func (buffer *boundedBuffer) String() string { return string(buffer.bytes) }

var _ io.Writer = (*boundedBuffer)(nil)
