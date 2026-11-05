package judge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

type fakeDocker struct {
	compiled  protocol.CompileResponse
	tests     []protocol.TestResponse
	requests  []protocol.TestRequest
	args      [][]string
	compiles  int
	phase     string
	oom       bool
	badExit   bool
	badJSON   bool
	failClean bool
}

func newFakeDocker(t *testing.T) (*DockerRunner, *fakeDocker) {
	t.Helper()
	fake := &fakeDocker{compiled: protocol.CompileResponse{Success: true, Files: []protocol.Artifact{{Path: "main.py", Data: []byte("print()")}}}}
	runner := &DockerRunner{image: "campusjudge-runner:local"}
	runner.execute = func(ctx context.Context, args []string, input []byte, limit int) (commandOutput, error) {
		fake.args = append(fake.args, slices.Clone(args))
		marshal := func(value any) commandOutput { data, _ := json.Marshal(value); return commandOutput{stdout: data} }
		switch args[0] {
		case "run":
			fake.phase = args[len(args)-1]
			if fake.phase == "compile" {
				fake.compiles++
				return marshal(fake.compiled), nil
			}
			var request protocol.TestRequest
			if err := json.Unmarshal(input, &request); err != nil {
				t.Fatal(err)
			}
			fake.requests = append(fake.requests, request)
			if fake.badJSON {
				return commandOutput{stdout: []byte("invalid")}, nil
			}
			if len(fake.tests) < len(fake.requests) {
				t.Fatal("unexpected additional test execution")
			}
			return marshal(fake.tests[len(fake.requests)-1]), nil
		case "inspect":
			state := containerState{Status: "exited", OOMKilled: fake.oom && fake.phase == "test"}
			if fake.badExit && fake.phase == "test" {
				state.ExitCode = 70
			}
			return marshal(state), nil
		case "rm":
			if fake.failClean {
				return commandOutput{}, errors.New("daemon disconnected")
			}
			return commandOutput{}, nil
		default:
			t.Fatalf("unexpected command: %v", args)
			return commandOutput{}, nil
		}
	}
	return runner, fake
}

func sampleJob() Job {
	return Job{Language: "python", Source: "print()", Tests: []model.TestCase{{Input: "input-one", Output: "5"}}, TimeLimitMS: 1000, MemoryLimitMB: 128}
}

func TestRunCompilesOnceAndComparesTokensOnHost(t *testing.T) {
	runner, fake := newFakeDocker(t)
	fake.tests = []protocol.TestResponse{{Stdout: []byte("  5\n"), TimeMS: 4}, {Stdout: []byte("7\t8\n"), TimeMS: 12}, {Stdout: []byte("99"), TimeMS: 8}}
	job := sampleJob()
	job.Tests = append(job.Tests, model.TestCase{Input: "input-two", Output: "7 8"}, model.TestCase{Input: "input-three", Output: "99"})
	result, err := runner.Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{Verdict: "accepted", TotalTests: 3, PassedTests: 3, TimeMS: 12}) || fake.compiles != 1 || len(fake.requests) != 3 {
		t.Fatalf("result=%+v compiles=%d tests=%d", result, fake.compiles, len(fake.requests))
	}
	for i, request := range fake.requests {
		if request.Input != job.Tests[i].Input {
			t.Fatal("runtime received another test's input")
		}
		encoded, _ := json.Marshal(request)
		if strings.Contains(string(encoded), "output") || strings.Contains(string(encoded), "input-three") && i != 2 {
			t.Fatal("runtime request contains expected output or another test")
		}
	}
	for _, args := range fake.args {
		if args[0] != "run" {
			continue
		}
		for _, required := range []string{"--network=none", "--read-only", "--cap-drop=ALL", "--cap-add=SETUID", "--cap-add=SETGID", "--cap-add=KILL", "--security-opt=no-new-privileges:true", "--pids-limit=128", "--cpus=1", "--log-driver=none"} {
			if !slices.Contains(args, required) {
				t.Fatalf("missing isolation option %s", required)
			}
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "--mount") || strings.HasPrefix(arg, "--volume") || strings.HasPrefix(arg, "--privileged") || strings.HasPrefix(arg, "--pid=host") {
				t.Fatalf("unsafe container option %s", arg)
			}
		}
		memory := "--memory=128m"
		if args[len(args)-1] == "compile" {
			memory = "--memory=1024m"
		}
		if !slices.Contains(args, memory) {
			t.Fatalf("incorrect phase memory: %v", args)
		}
	}
}

func TestWrongAnswerStopsAfterFailedTestWithoutDisclosingData(t *testing.T) {
	runner, fake := newFakeDocker(t)
	fake.tests = []protocol.TestResponse{{Stdout: []byte("5")}, {Stdout: []byte("actual-private-result")}}
	job := sampleJob()
	job.Tests = append(job.Tests, model.TestCase{Input: "hidden-private-input", Output: "expected-private-result"}, model.TestCase{Output: "not-run"})
	result, err := runner.Run(context.Background(), job)
	if err != nil || result.Verdict != "wrong_answer" || result.PassedTests != 1 || result.FailedTest != 2 || len(fake.requests) != 2 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "private") {
		t.Fatal("result disclosed hidden test contents")
	}
}

func TestExecutionVerdicts(t *testing.T) {
	cases := []struct {
		name     string
		response protocol.TestResponse
		oom      bool
	}{
		{"runtime_error", protocol.TestResponse{ExitCode: 2}, false},
		{"time_limit", protocol.TestResponse{TimedOut: true, ExitCode: -1}, false},
		{"memory_limit", protocol.TestResponse{MemoryExceeded: true, ExitCode: -1}, false},
		{"memory_limit", protocol.TestResponse{}, true},
		{"output_limit", protocol.TestResponse{OutputLimited: true, ExitCode: -1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runner, fake := newFakeDocker(t)
			fake.tests, fake.oom = []protocol.TestResponse{tc.response}, tc.oom
			result, err := runner.Run(context.Background(), sampleJob())
			if err != nil || result.Verdict != tc.name || result.FailedTest != 1 || result.PassedTests != 0 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestCompilationErrorIsCappedAndRunsNoTests(t *testing.T) {
	runner, fake := newFakeDocker(t)
	fake.compiled = protocol.CompileResponse{Log: strings.Repeat("x", protocol.MaxCompilerLogBytes+100)}
	result, err := runner.Run(context.Background(), sampleJob())
	if err != nil || result.Verdict != "compilation_error" || result.TotalTests != 1 || len(result.CompileOutput) != protocol.MaxCompilerLogBytes || len(fake.requests) != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}

func TestInfrastructureFailuresAreNotStudentVerdicts(t *testing.T) {
	for _, failure := range []string{"compile", "runtime", "json", "exit", "cleanup", "artifacts"} {
		t.Run(failure, func(t *testing.T) {
			runner, fake := newFakeDocker(t)
			fake.tests = []protocol.TestResponse{{Stdout: []byte("5")}}
			switch failure {
			case "compile":
				fake.compiled.Error = "compiler missing"
			case "runtime":
				fake.tests[0].Error = "cgroup unavailable"
			case "json":
				fake.badJSON = true
			case "exit":
				fake.badExit = true
			case "cleanup":
				fake.failClean = true
			case "artifacts":
				fake.compiled.Files[0].Path = "../outside"
			}
			result, err := runner.Run(context.Background(), sampleJob())
			if err == nil || result.Verdict == "wrong_answer" || result.Verdict == "accepted" {
				t.Fatalf("infrastructure failure became a student result: %+v %v", result, err)
			}
		})
	}
}

func TestCancellationStillUsesIndependentCleanupContext(t *testing.T) {
	runner := &DockerRunner{image: "image"}
	ctx, cancel := context.WithCancel(context.Background())
	cleaned := false
	runner.execute = func(commandCtx context.Context, args []string, _ []byte, _ int) (commandOutput, error) {
		if args[0] == "run" {
			cancel()
			return commandOutput{}, commandCtx.Err()
		}
		if args[0] == "rm" {
			cleaned = commandCtx.Err() == nil
			return commandOutput{}, nil
		}
		t.Fatalf("unexpected command %v", args)
		return commandOutput{}, nil
	}
	_, err := runner.Run(ctx, sampleJob())
	if !errors.Is(err, context.Canceled) || !cleaned {
		t.Fatalf("cancel=%v cleaned=%v", err, cleaned)
	}
}

func TestInvalidJobsNeverStartAContainer(t *testing.T) {
	cases := []func(*Job){
		func(j *Job) { j.Language = "unknown" },
		func(j *Job) { j.Source = "" },
		func(j *Job) { j.Tests = nil },
		func(j *Job) { j.TimeLimitMS = 0 },
		func(j *Job) { j.MemoryLimitMB = 0 },
		func(j *Job) { j.Tests[0].Input = strings.Repeat("x", protocol.MaxTestBytes+1) },
	}
	for i, change := range cases {
		job := sampleJob()
		change(&job)
		runner, fake := newFakeDocker(t)
		if _, err := runner.Run(context.Background(), job); err == nil || len(fake.args) != 0 {
			t.Fatalf("case %d started invalid job", i)
		}
	}
}

func TestBoundedCaptureDiscardsExcessBytes(t *testing.T) {
	buffer := &boundedBuffer{limit: 3}
	_, _ = buffer.Write([]byte("ab"))
	n, err := buffer.Write([]byte("cdef"))
	if n != 4 || err != nil || buffer.String() != "abc" || !buffer.overflow {
		t.Fatalf("capture=%q overflow=%v write=%d,%v", buffer.String(), buffer.overflow, n, err)
	}
	// Exercise io.Copy's ReaderFrom fast path as well: an embedded bytes.Buffer
	// would bypass Write and silently remove the memory bound.
	second := &boundedBuffer{limit: 3}
	_, _ = io.Copy(second, struct{ io.Reader }{strings.NewReader("abcdefgh")})
	if second.String() != "abc" || !second.overflow {
		t.Fatal("io.Copy bypassed the capture limit")
	}
}

func TestPrivateResponseRequiresExactlyOneKnownDocument(t *testing.T) {
	for _, input := range []string{`{"success":true} {}`, `{"success":true,"unknown":1}`, `not-json`} {
		if err := decodeResponse([]byte(input), new(protocol.CompileResponse)); err == nil {
			t.Fatalf("accepted malformed protocol %s", input)
		}
	}
	for _, language := range []string{"c", "cpp", "python", "go", "java", "javascript"} {
		if !Supports(language) {
			t.Fatalf("missing promised language %s", language)
		}
	}
	for _, language := range []string{"other", "csharp", "python; command"} {
		if Supports(language) {
			t.Fatalf("unconfigured language became automatic: %s", language)
		}
	}
}

func TestCheckRequiresAllResourceControlsBeforeAcceptingDaemon(t *testing.T) {
	cases := []struct {
		name        string
		change      func(map[string]any)
		invalidJSON bool
		overflow    bool
		infoError   bool
		wantError   bool
	}{
		{name: "supported cgroup v2"},
		{name: "cgroup v1", change: func(info map[string]any) { info["CgroupVersion"] = "1" }, wantError: true},
		{name: "missing cgroup", change: func(info map[string]any) { delete(info, "CgroupVersion") }, wantError: true},
		{name: "memory unavailable", change: func(info map[string]any) { info["MemoryLimit"] = false }, wantError: true},
		{name: "swap unavailable", change: func(info map[string]any) { info["SwapLimit"] = false }, wantError: true},
		{name: "PID control unavailable", change: func(info map[string]any) { info["PidsLimit"] = false }, wantError: true},
		{name: "CPU quota unavailable", change: func(info map[string]any) { info["CpuCfsQuota"] = false }, wantError: true},
		{name: "missing capability", change: func(info map[string]any) { delete(info, "MemoryLimit") }, wantError: true},
		{name: "malformed daemon response", invalidJSON: true, wantError: true},
		{name: "truncated daemon response", overflow: true, wantError: true},
		{name: "daemon disconnected during inspection", infoError: true, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := map[string]any{"CgroupVersion": "2", "MemoryLimit": true, "SwapLimit": true, "PidsLimit": true, "CpuCfsQuota": true, "UnrelatedDaemonField": "ignored"}
			if tc.change != nil {
				tc.change(info)
			}
			encoded, _ := json.Marshal(info)
			if tc.invalidJSON {
				encoded = []byte("not JSON")
			}
			checkedImage := false
			runner := &DockerRunner{image: "image"}
			runner.execute = func(_ context.Context, args []string, _ []byte, _ int) (commandOutput, error) {
				switch args[0] {
				case "version":
					return commandOutput{stdout: []byte("linux\n")}, nil
				case "info":
					if tc.infoError {
						return commandOutput{}, errors.New("disconnected")
					}
					return commandOutput{stdout: encoded, overflow: tc.overflow}, nil
				case "image":
					checkedImage = true
					return commandOutput{stdout: []byte("image-id")}, nil
				default:
					t.Fatalf("Check started an unexpected command: %v", args)
					return commandOutput{}, nil
				}
			}
			if err := runner.Check(context.Background()); (err != nil) != tc.wantError {
				t.Fatalf("Check error=%v, wantError=%v", err, tc.wantError)
			}
			if checkedImage == tc.wantError {
				t.Fatal("unsupported resource controls did not stop startup before image acceptance")
			}
		})
	}
}

func TestDockerIntegrationCheckOnly(t *testing.T) {
	// integrationRunner checks the actual configured daemon and local image;
	// this test intentionally starts no compiler or submitted program.
	_, _ = integrationRunner(t)
}
