// Package protocol defines the bounded private supervisor protocol. Expected
// outputs never enter containers.
package protocol

import (
	"errors"
	"path"
	"strings"
)

const (
	MaxSourceBytes          = 128 * 1024
	MaxTestBytes            = 64 * 1024
	MaxTests                = 120
	MaxOutputBytes          = 1024 * 1024
	MaxCompilerLogBytes     = 16 * 1024
	MaxArtifactBytes        = 32 * 1024 * 1024
	MaxArtifactFiles        = 256
	MaxCompileResponseBytes = 48 * 1024 * 1024
	MaxTestRequestBytes     = 48 * 1024 * 1024
	MaxTestResponseBytes    = 2 * 1024 * 1024
)

type Artifact struct {
	Path       string `json:"path"`
	Data       []byte `json:"data"`
	Executable bool   `json:"executable,omitempty"`
}

type CompileRequest struct {
	Language string `json:"language"`
	Source   string `json:"source"`
}

type CompileResponse struct {
	Success bool       `json:"success"`
	Files   []Artifact `json:"files,omitempty"`
	Log     string     `json:"log,omitempty"`
	Error   string     `json:"error,omitempty"`
}

type TestRequest struct {
	Language      string     `json:"language"`
	Files         []Artifact `json:"files"`
	Input         string     `json:"input"`
	TimeLimitMS   int        `json:"time_limit_ms"`
	MemoryLimitMB int        `json:"memory_limit_mb"`
}

type TestResponse struct {
	Stdout         []byte `json:"stdout"`
	ExitCode       int    `json:"exit_code"`
	TimeMS         int64  `json:"time_ms"`
	TimedOut       bool   `json:"timed_out,omitempty"`
	OutputLimited  bool   `json:"output_limited,omitempty"`
	MemoryExceeded bool   `json:"memory_exceeded,omitempty"`
	Error          string `json:"error,omitempty"`
}

type LanguageSpec struct {
	SourceFile string
	Compile    []string
	Run        []string
}

// Commands are fixed by the image, never supplied by an HTTP request.
func Language(id string) (LanguageSpec, bool) {
	switch id {
	case "cpp":
		return LanguageSpec{"main.cpp", []string{"g++", "-std=c++20", "-O2", "-pipe", "-o", "/work/program", "/work/main.cpp"}, []string{"/work/program"}}, true
	case "c":
		return LanguageSpec{"main.c", []string{"gcc", "-std=c17", "-O2", "-pipe", "-o", "/work/program", "/work/main.c"}, []string{"/work/program"}}, true
	case "python":
		return LanguageSpec{"main.py", []string{"python3", "-I", "-B", "-c", "import ast,pathlib; ast.parse(pathlib.Path('/work/main.py').read_text())"}, []string{"python3", "-I", "-B", "/work/main.py"}}, true
	case "go":
		return LanguageSpec{"main.go", []string{"go", "build", "-trimpath", "-o", "/work/program", "/work/main.go"}, []string{"/work/program"}}, true
	case "java":
		return LanguageSpec{"Main.java", []string{"javac", "-J-Xmx512m", "-J-XX:ActiveProcessorCount=1", "-encoding", "UTF-8", "-d", "/work", "/work/Main.java"}, []string{"java", "-XX:ActiveProcessorCount=1", "-XX:-UsePerfData", "-XX:CompressedClassSpaceSize=32m", "-XX:ReservedCodeCacheSize=32m", "-Xss256k", "-Xms8m", "-cp", "/work", "Main"}}, true
	case "javascript":
		return LanguageSpec{"main.js", []string{"node", "--check", "/work/main.js"}, []string{"node", "/work/main.js"}}, true
	default:
		return LanguageSpec{}, false
	}
}

func ValidateArtifacts(files []Artifact) error {
	if len(files) == 0 || len(files) > MaxArtifactFiles {
		return errors.New("invalid artifact count")
	}
	seen := make(map[string]bool)
	var size int64
	for _, file := range files {
		if file.Path == "" || len(file.Path) > 256 || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path ||
			file.Path == "." || file.Path == ".." || strings.HasPrefix(file.Path, "../") || strings.ContainsAny(file.Path, "\\\x00") || seen[file.Path] {
			return errors.New("invalid artifact path")
		}
		seen[file.Path] = true
		size += int64(len(file.Data))
		if size > MaxArtifactBytes {
			return errors.New("compiled artifacts exceed 32 MiB")
		}
	}
	return nil
}
