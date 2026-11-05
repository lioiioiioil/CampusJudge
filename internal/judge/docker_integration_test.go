package judge

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

// Opt in explicitly: CAMPUSJUDGE_DOCKER_TESTS=1 go test -v ./internal/judge.
// Submitted fixture programs execute only in the disposable Docker containers.
func integrationRunner(t *testing.T) (*DockerRunner, *[]string) {
	t.Helper()
	if os.Getenv("CAMPUSJUDGE_DOCKER_TESTS") != "1" {
		t.Skip("set CAMPUSJUDGE_DOCKER_TESTS=1 and build judge/Dockerfile to exercise real Docker isolation")
	}
	runner, err := NewDocker(DockerOptions{Binary: os.Getenv("CAMPUSJUDGE_DOCKER_BINARY"), Image: os.Getenv("CAMPUSJUDGE_JUDGE_IMAGE")})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	runner.execute = func(ctx context.Context, args []string, input []byte, limit int) (commandOutput, error) {
		if args[0] == "run" {
			for i, arg := range args {
				if arg == "--name" {
					names = append(names, args[i+1])
				}
			}
		}
		return runner.command(ctx, args, input, limit)
	}
	t.Cleanup(func() {
		for _, name := range names {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			output, err := exec.CommandContext(ctx, runner.binary, "ps", "--all", "--filter", "name=^/"+name+"$", "--format", "{{.Names}}").Output()
			cancel()
			if err != nil || len(strings.TrimSpace(string(output))) != 0 {
				t.Errorf("sandbox cleanup not verified for %s: %q %v", name, output, err)
			}
		}
	})
	return runner, &names
}

func TestDockerIntegrationLanguages(t *testing.T) {
	programs := []struct{ language, source string }{
		{"cpp", "#include <iostream>\nint main(){long a,b;std::cin>>a>>b;std::cout<<a+b;}"},
		{"c", "#include <stdio.h>\nint main(){long a,b;scanf(\"%ld%ld\",&a,&b);printf(\"%ld\\n\",a+b);}"},
		{"python", "a,b=map(int,input().split()); print(a+b)"},
		{"go", "package main\nimport(\"fmt\";\"os\")\nfunc main(){var a,b int;fmt.Fscan(os.Stdin,&a,&b);fmt.Println(a+b)}"},
		{"java", "import java.util.Scanner; public class Main { public static void main(String[] a){Scanner s=new Scanner(System.in);System.out.println(s.nextLong()+s.nextLong());}}"},
		{"javascript", "const n=require('fs').readFileSync(0,'utf8').trim().split(/\\s+/).map(Number); console.log(n[0]+n[1]);"},
	}
	for _, program := range programs {
		t.Run(program.language, func(t *testing.T) {
			runner, _ := integrationRunner(t)
			job := Job{Language: program.language, Source: program.source, Tests: []model.TestCase{{Input: "2 3\n", Output: "5\n"}, {Input: "-5 8\n", Output: "3\n"}}, TimeLimitMS: 2000, MemoryLimitMB: 256}
			result, err := runner.Run(context.Background(), job)
			if err != nil || result.Verdict != "accepted" || result.PassedTests != 2 {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestDockerIntegrationVerdicts(t *testing.T) {
	cases := []struct{ verdict, language, source string }{
		{"wrong_answer", "python", "print(0)"},
		{"compilation_error", "python", "def broken(:"},
		{"runtime_error", "python", "raise RuntimeError('intentional fixture failure')"},
		{"time_limit", "python", "while True: pass"},
		{"output_limit", "python", "import os\nwhile True: os.write(1,b'x'*65536)"},
		{"memory_limit", "c", "#include <stdlib.h>\nint main(){for(;;){volatile char*p=malloc(16777216);if(!p)return 2;for(int i=0;i<16777216;i+=4096)p[i]=1;}}"},
	}
	for _, tc := range cases {
		t.Run(tc.verdict, func(t *testing.T) {
			runner, _ := integrationRunner(t)
			job := sampleJob()
			job.Language, job.Source, job.MemoryLimitMB, job.TimeLimitMS = tc.language, tc.source, 64, 1000
			result, err := runner.Run(context.Background(), job)
			if err != nil || result.Verdict != tc.verdict {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestDockerIntegrationIsolationAndFreshWorkspace(t *testing.T) {
	runner, _ := integrationRunner(t)
	job := sampleJob()
	job.MemoryLimitMB = 256
	job.Tests = []model.TestCase{{Output: "5"}, {Output: "5"}}
	job.Source = `import os
assert os.getuid() == 65534
assert os.getgroups() == []
status = open('/proc/self/status').read().splitlines()
caps = next(line.split()[1] for line in status if line.startswith('CapEff:'))
assert int(caps, 16) == 0
assert set(os.listdir('/sys/class/net')) == {'lo'}
assert not os.path.exists('/var/run/docker.sock')
assert open('/sys/fs/cgroup/pids.max').read().strip() == '128'
assert int(open('/sys/fs/cgroup/memory.max').read()) == 256*1024*1024
try:
    os.open('/proc/1/fd/1', os.O_WRONLY)
except PermissionError:
    pass
else:
    raise RuntimeError('supervisor protocol is writable')
try:
    open('/etc/campusjudge-write-probe', 'w')
except OSError:
    pass
else:
    raise RuntimeError('image root is writable')
assert not os.path.exists('/work/test-marker')
open('/work/test-marker', 'w').write('one test only')
print(5)
`
	result, err := runner.Run(context.Background(), job)
	if err != nil || result.Verdict != "accepted" || result.PassedTests != 2 {
		t.Fatalf("isolation probe result=%+v error=%v", result, err)
	}
}

func TestDockerIntegrationCancellationRemovesRunningContainer(t *testing.T) {
	runner, _ := integrationRunner(t)
	started := make(chan string, 1)
	execute := runner.execute
	runner.execute = func(ctx context.Context, args []string, input []byte, limit int) (commandOutput, error) {
		if args[0] == "run" && args[len(args)-1] == "test" {
			for i, arg := range args {
				if arg == "--name" {
					started <- args[i+1]
				}
			}
		}
		return execute(ctx, args, input, limit)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	job := sampleJob()
	job.Source, job.TimeLimitMS = "while True: pass", 30000
	go func() { _, err := runner.Run(ctx, job); done <- err }()
	var name string
	select {
	case name = <-started:
	case err := <-done:
		t.Fatalf("job stopped before runtime container: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("runtime did not start")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		inspectCtx, stop := context.WithTimeout(context.Background(), time.Second)
		output, _ := exec.CommandContext(inspectCtx, runner.binary, "inspect", "--format", "{{.State.Running}}", name).Output()
		stop()
		if strings.TrimSpace(string(output)) == "true" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("container did not become running")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation returned %v", err)
		}
	case <-time.After(12 * time.Second):
		t.Fatal("cancel did not finish cleanup")
	}
}

func TestDockerIntegrationIncompleteRequestWatchdog(t *testing.T) {
	runner, names := integrationRunner(t)
	name := "campusjudge-watchdog-" + strings.ToLower(rand.Text()[:16])
	*names = append(*names, name)
	args := runner.containerArgs(name, "test", 128)
	args[0] = "create"
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		if _, err := runner.command(cleanupCtx, []string{"rm", "--force", name}, nil, 4096); err != nil {
			t.Error("watchdog container cleanup failed:", err)
		}
	}()
	if _, err := runner.command(ctx, args, nil, 4096); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	// OpenStdin remains open without any writer. This models an API crash or
	// truncated upload before the trusted supervisor can read a complete job.
	if _, err := runner.command(ctx, []string{"start", name}, nil, 4096); err != nil {
		t.Fatal(err)
	}
	output, err := runner.command(ctx, []string{"wait", name}, nil, 4096)
	if err != nil || strings.TrimSpace(string(output.stdout)) != "70" || time.Since(started) > 60*time.Second {
		t.Fatalf("absolute guest watchdog: exit=%q elapsed=%s error=%v", output.stdout, time.Since(started), err)
	}
}
