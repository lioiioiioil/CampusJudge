//go:build linux

// This supervisor is trusted image code. It never runs on the API host.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
)

func main() {
	if len(os.Args) != 2 || os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "invalid supervisor invocation")
		os.Exit(1)
	}
	// This watchdog starts before reading stdin. Even if the API is killed or
	// stops sending a request, the guest cannot outlive this absolute deadline.
	watchdogLimit := 45 * time.Second
	if os.Args[1] == "compile" {
		watchdogLimit = 75 * time.Second
	}
	watchdog := time.AfterFunc(watchdogLimit, func() { os.Exit(70) })
	defer watchdog.Stop()
	switch os.Args[1] {
	case "compile":
		_ = json.NewEncoder(os.Stdout).Encode(compile())
	case "test":
		_ = json.NewEncoder(os.Stdout).Encode(runTest())
	default:
		fmt.Fprintln(os.Stderr, "invalid supervisor phase")
		os.Exit(1)
	}
}

func readRequest(limit int, value any) error {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, int64(limit)+1))
	if err != nil || len(data) > limit {
		return errors.New("request exceeds its hard limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return errors.New("invalid supervisor request")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("invalid supervisor request suffix")
	}
	return nil
}

func childCommand(ctx context.Context, args []string) *exec.Cmd {
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Dir = "/work"
	command.Env = []string{"PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8", "HOME=/tmp", "GOCACHE=/tmp/go-cache", "GOPATH=/tmp/go", "GOPROXY=off", "GOSUMDB=off", "CGO_ENABLED=0", "GOMAXPROCS=1"}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 65534, Gid: 65534, Groups: []uint32{}}}
	command.WaitDelay = 250 * time.Millisecond
	return command
}

func compile() protocol.CompileResponse {
	var request protocol.CompileRequest
	if err := readRequest(protocol.MaxSourceBytes*6+4096, &request); err != nil {
		return protocol.CompileResponse{Error: err.Error()}
	}
	language, supported := protocol.Language(request.Language)
	if !supported || len(request.Source) == 0 || len(request.Source) > protocol.MaxSourceBytes {
		return protocol.CompileResponse{Error: "invalid compile request"}
	}
	if err := os.WriteFile(filepath.Join("/work", language.SourceFile), []byte(request.Source), 0o644); err != nil {
		return protocol.CompileResponse{Error: "cannot prepare source file"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	log := &outputBudget{limit: protocol.MaxCompilerLogBytes, cancel: cancel}
	command := childCommand(ctx, language.Compile)
	command.Stdout, command.Stderr = log.writer(false), log.writer(false)
	err := command.Run()
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) && ctx.Err() == nil && !log.exceeded {
			return protocol.CompileResponse{Error: "compiler could not be started"}
		}
		message := string(log.stderr.Bytes())
		if ctx.Err() != nil && message == "" {
			message = "Compilation exceeded its time or diagnostic output limit."
		}
		return protocol.CompileResponse{Log: message}
	}
	files, err := collectArtifacts()
	if err != nil {
		return protocol.CompileResponse{Log: "Compiled artifacts exceed workspace limits."}
	}
	return protocol.CompileResponse{Success: true, Files: files}
}

func collectArtifacts() ([]protocol.Artifact, error) {
	files := make([]protocol.Artifact, 0)
	var total int64
	err := filepath.WalkDir("/work", func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || len(files) >= protocol.MaxArtifactFiles {
			return errors.New("invalid compile artifact")
		}
		if info.Size() > protocol.MaxArtifactBytes-total {
			return errors.New("compile artifacts too large")
		}
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, protocol.MaxArtifactBytes-total+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		total += int64(len(data))
		if total > protocol.MaxArtifactBytes {
			return errors.New("compile artifacts grew beyond their hard limit")
		}
		relative, _ := filepath.Rel("/work", filename)
		files = append(files, protocol.Artifact{Path: filepath.ToSlash(relative), Data: data, Executable: info.Mode().Perm()&0o111 != 0})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, protocol.ValidateArtifacts(files)
}

func runTest() protocol.TestResponse {
	var request protocol.TestRequest
	if err := readRequest(protocol.MaxTestRequestBytes, &request); err != nil {
		return protocol.TestResponse{Error: err.Error()}
	}
	language, supported := protocol.Language(request.Language)
	if !supported || request.TimeLimitMS < 100 || request.TimeLimitMS > 30000 || request.MemoryLimitMB < 16 || request.MemoryLimitMB > 2048 || len(request.Input) > protocol.MaxTestBytes {
		return protocol.TestResponse{Error: "invalid runtime request"}
	}
	if err := protocol.ValidateArtifacts(request.Files); err != nil {
		return protocol.TestResponse{Error: "invalid runtime artifacts"}
	}
	for _, file := range request.Files {
		filename := filepath.Join("/work", file.Path)
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			return protocol.TestResponse{Error: "cannot prepare artifact directory"}
		}
		mode := fs.FileMode(0o644)
		if file.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(filename, file.Data, mode); err != nil {
			return protocol.TestResponse{Error: "cannot prepare runtime artifact"}
		}
	}
	beforeOOM, err := oomKills()
	if err != nil {
		return protocol.TestResponse{Error: "cgroup v2 memory accounting is unavailable"}
	}
	args := language.Run
	if request.Language == "java" {
		args = append([]string{args[0], "-Xmx" + strconv.Itoa(max(8, request.MemoryLimitMB/2)) + "m"}, args[1:]...)
	}
	if request.Language == "javascript" {
		args = append([]string{args[0], "--max-old-space-size=" + strconv.Itoa(max(8, request.MemoryLimitMB*3/5))}, args[1:]...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(request.TimeLimitMS)*time.Millisecond)
	defer cancel()
	output := &outputBudget{limit: protocol.MaxOutputBytes, cancel: cancel}
	command := childCommand(ctx, args)
	command.Stdin = strings.NewReader(request.Input)
	command.Stdout, command.Stderr = output.writer(true), output.writer(false)
	started := time.Now()
	err = command.Run()
	result := protocol.TestResponse{Stdout: output.stdout.Bytes(), TimeMS: time.Since(started).Milliseconds(), OutputLimited: output.exceeded, TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded)}
	if command.ProcessState != nil {
		result.ExitCode = command.ProcessState.ExitCode()
	}
	if afterOOM, readErr := oomKills(); readErr != nil {
		return protocol.TestResponse{Error: "cannot read cgroup memory accounting"}
	} else {
		result.MemoryExceeded = afterOOM > beforeOOM
	}
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) && !errors.Is(err, exec.ErrWaitDelay) && ctx.Err() == nil {
			return protocol.TestResponse{Error: "solution process could not be started"}
		}
	}
	return result
}

func oomKills() (int64, error) {
	data, err := os.ReadFile("/sys/fs/cgroup/memory.events")
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[0] == "oom_kill" {
			return strconv.ParseInt(parts[1], 10, 64)
		}
	}
	return 0, errors.New("oom_kill counter is missing")
}

// stdout and stderr share a budget. The child never inherits PID 1's protocol
// stream: each stream goes through its own pipe into this bounded collector.
type outputBudget struct {
	mu       sync.Mutex
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	limit    int
	written  int
	exceeded bool
	cancel   context.CancelFunc
}

type budgetWriter struct {
	budget *outputBudget
	stdout bool
}

func (budget *outputBudget) writer(stdout bool) io.Writer { return budgetWriter{budget, stdout} }

func (writer budgetWriter) Write(data []byte) (int, error) {
	budget := writer.budget
	budget.mu.Lock()
	defer budget.mu.Unlock()
	remaining := max(0, budget.limit-budget.written)
	part := data[:min(remaining, len(data))]
	if writer.stdout {
		_, _ = budget.stdout.Write(part)
	} else {
		_, _ = budget.stderr.Write(part)
	}
	budget.written += len(part)
	if len(data) > remaining {
		budget.exceeded = true
		budget.cancel()
	}
	return len(data), nil
}
