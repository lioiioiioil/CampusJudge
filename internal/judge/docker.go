package judge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
)

type DockerOptions struct {
	Binary string
	Image  string
}

type DockerRunner struct {
	binary  string
	image   string
	execute commandFunc
}

type commandOutput struct {
	stdout   []byte
	stderr   string
	overflow bool
}

type commandFunc func(context.Context, []string, []byte, int) (commandOutput, error)

type containerState struct {
	OOMKilled bool
	ExitCode  int
	Status    string
}

func NewDocker(options DockerOptions) (*DockerRunner, error) {
	if options.Binary == "" {
		options.Binary = "docker"
	}
	if options.Image == "" {
		options.Image = "campusjudge-runner:local"
	}
	if strings.HasPrefix(options.Image, "-") || strings.ContainsAny(options.Image, " \t\r\n\x00") {
		return nil, errors.New("invalid judge image reference")
	}
	binary, err := exec.LookPath(options.Binary)
	if err != nil {
		return nil, fmt.Errorf("locate Docker CLI: %w", err)
	}
	runner := &DockerRunner{binary: binary, image: options.Image}
	runner.execute = runner.command
	return runner, nil
}

func (runner *DockerRunner) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := runner.execute(ctx, []string{"version", "--format", "{{.Server.Os}}"}, nil, 4096)
	if err != nil {
		return fmt.Errorf("Docker daemon unavailable: %w", err)
	}
	if strings.TrimSpace(string(output.stdout)) != "linux" {
		return errors.New("judge requires a Linux Docker daemon")
	}
	if _, err := runner.execute(ctx, []string{"image", "inspect", runner.image, "--format", "{{.Id}}"}, nil, 4096); err != nil {
		return fmt.Errorf("judge image is unavailable; build judge/Dockerfile first: %w", err)
	}
	return nil
}

func (runner *DockerRunner) Run(ctx context.Context, job Job) (Result, error) {
	result := Result{TotalTests: len(job.Tests)}
	if err := validateJob(job); err != nil {
		return result, err
	}
	request, err := json.Marshal(protocol.CompileRequest{Language: job.Language, Source: job.Source})
	if err != nil {
		return result, err
	}
	output, state, err := runner.container(ctx, "compile", request, 1024, 80*time.Second, protocol.MaxCompileResponseBytes)
	if err != nil {
		return result, err
	}
	if state.OOMKilled {
		result.Verdict, result.CompileOutput = "compilation_error", "Compilation exceeded its memory limit."
		return result, nil
	}
	var compiled protocol.CompileResponse
	if err := decodeResponse(output, &compiled); err != nil {
		return result, fmt.Errorf("invalid compiler supervisor response: %w", err)
	}
	if compiled.Error != "" {
		return result, fmt.Errorf("compiler infrastructure: %s", compiled.Error)
	}
	result.CompileOutput = compiled.Log[:min(len(compiled.Log), protocol.MaxCompilerLogBytes)]
	if !compiled.Success {
		result.Verdict = "compilation_error"
		return result, nil
	}
	if err := protocol.ValidateArtifacts(compiled.Files); err != nil {
		return result, fmt.Errorf("invalid compiler artifact package: %w", err)
	}
	for index, test := range job.Tests {
		// Only this input enters the container. Expected output and every other
		// hidden test remain in the trusted host process.
		request, err := json.Marshal(protocol.TestRequest{Language: job.Language, Files: compiled.Files, Input: test.Input, TimeLimitMS: job.TimeLimitMS, MemoryLimitMB: job.MemoryLimitMB})
		if err != nil {
			return result, err
		}
		if len(request) > protocol.MaxTestRequestBytes {
			return result, errors.New("runtime request exceeds its hard limit")
		}
		output, state, err := runner.container(ctx, "test", request, job.MemoryLimitMB, time.Duration(job.TimeLimitMS)*time.Millisecond+20*time.Second, protocol.MaxTestResponseBytes)
		if err != nil {
			return result, err
		}
		verdict := ""
		if state.OOMKilled {
			verdict = "memory_limit"
		} else {
			var executed protocol.TestResponse
			if err := decodeResponse(output, &executed); err != nil {
				return result, fmt.Errorf("invalid runtime supervisor response: %w", err)
			}
			if executed.Error != "" {
				return result, fmt.Errorf("runtime infrastructure: %s", executed.Error)
			}
			if executed.TimeMS < 0 || len(executed.Stdout) > protocol.MaxOutputBytes {
				return result, errors.New("invalid runtime measurements")
			}
			result.TimeMS = max(result.TimeMS, executed.TimeMS)
			switch {
			case executed.MemoryExceeded:
				verdict = "memory_limit"
			case executed.OutputLimited:
				verdict = "output_limit"
			case executed.TimedOut:
				verdict = "time_limit"
			case executed.ExitCode != 0:
				verdict = "runtime_error"
			case !slices.Equal(strings.Fields(string(executed.Stdout)), strings.Fields(test.Output)):
				verdict = "wrong_answer"
			}
		}
		if verdict != "" {
			result.Verdict, result.FailedTest = verdict, index+1
			return result, nil
		}
		result.PassedTests++
	}
	result.Verdict = "accepted"
	return result, nil
}

func (runner *DockerRunner) container(ctx context.Context, phase string, input []byte, memoryMB int, timeout time.Duration, outputLimit int) (output []byte, state containerState, err error) {
	name := "campusjudge-" + strings.ToLower(rand.Text()[:20])
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		removed, removeErr := runner.execute(cleanupCtx, []string{"rm", "--force", name}, nil, 4096)
		if removeErr != nil && !strings.Contains(removed.stderr, "No such container") {
			err = errors.Join(err, errors.New("could not confirm sandbox container cleanup"))
		}
	}()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	runOutput, runErr := runner.execute(runCtx, runner.containerArgs(name, phase, memoryMB), input, outputLimit)
	if ctx.Err() != nil {
		return nil, state, ctx.Err()
	}
	if runCtx.Err() != nil {
		return nil, state, errors.New("sandbox supervisor did not finish within its outer deadline")
	}
	inspectCtx, inspectCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer inspectCancel()
	inspected, inspectErr := runner.execute(inspectCtx, []string{"inspect", "--format", "{{json .State}}", name}, nil, 16384)
	if inspectErr != nil {
		return nil, state, fmt.Errorf("cannot verify sandbox exit: %w", errors.Join(runErr, inspectErr))
	}
	if err := json.Unmarshal(inspected.stdout, &state); err != nil {
		return nil, state, errors.New("Docker returned an invalid container state")
	}
	if state.OOMKilled {
		return nil, state, nil
	}
	if runErr != nil || state.ExitCode != 0 || state.Status != "exited" {
		if runErr != nil {
			return nil, state, fmt.Errorf("sandbox supervisor failed (container exit %d): %w", state.ExitCode, runErr)
		}
		return nil, state, fmt.Errorf("sandbox supervisor failed (container exit %d)", state.ExitCode)
	}
	if runOutput.overflow {
		return nil, state, errors.New("sandbox response exceeds its hard limit")
	}
	return runOutput.stdout, state, nil
}

func (runner *DockerRunner) containerArgs(name, phase string, memoryMB int) []string {
	// Trusted PID 1 alone keeps SETUID/SETGID to launch uid/gid 65534 with no
	// supplementary groups or capabilities, and KILL to enforce child timeouts.
	// Different UIDs prevent /proc/1/fd/1 access and forged supervisor messages.
	return []string{"run", "--interactive", "--name", name, "--label=campusjudge.sandbox=true", "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--cap-add=SETUID", "--cap-add=SETGID", "--cap-add=KILL", "--security-opt=no-new-privileges:true", "--pids-limit=128", "--cpus=1", "--memory=" + strconv.Itoa(memoryMB) + "m", "--memory-swap=" + strconv.Itoa(memoryMB) + "m", "--ulimit=nofile=128:128", "--ulimit=fsize=67108864:67108864", "--ulimit=core=0:0", "--log-driver=none", "--tmpfs=/work:rw,exec,nosuid,nodev,size=128m,mode=1777", "--tmpfs=/tmp:rw,noexec,nosuid,nodev,size=128m,mode=1777", "--workdir=/work", "--user=0:0", "--entrypoint=/usr/local/bin/campusjudge-sandbox", runner.image, phase}
}

func decodeResponse(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("unexpected extra supervisor output")
	}
	return nil
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (buffer *boundedBuffer) Bytes() []byte  { return buffer.buffer.Bytes() }
func (buffer *boundedBuffer) String() string { return buffer.buffer.String() }

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	remaining := max(0, buffer.limit-buffer.buffer.Len())
	if len(data) > remaining {
		buffer.overflow = true
	}
	_, _ = buffer.buffer.Write(data[:min(remaining, len(data))])
	return len(data), nil
}

func (runner *DockerRunner) command(ctx context.Context, args []string, input []byte, outputLimit int) (commandOutput, error) {
	command := exec.CommandContext(ctx, runner.binary, args...)
	command.Stdin = bytes.NewReader(input)
	stdout := &boundedBuffer{limit: outputLimit}
	stderr := &boundedBuffer{limit: protocol.MaxCompilerLogBytes}
	command.Stdout, command.Stderr = stdout, stderr
	command.WaitDelay = 500 * time.Millisecond
	err := command.Run()
	return commandOutput{stdout: stdout.Bytes(), stderr: stderr.String(), overflow: stdout.overflow}, err
}
