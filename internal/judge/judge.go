// Package judge runs submitted programs exclusively in disposable Linux containers.
package judge

import (
	"context"
	"errors"

	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

var ErrUnsupportedLanguage = errors.New("language has no automatic runner")

type Job struct {
	Language      string
	Source        string
	Tests         []model.TestCase
	TimeLimitMS   int
	MemoryLimitMB int
}

// Result never contains test input or actual/expected output. FailedTest is
// one-based, or zero when no individual test failed. TimeMS is the maximum wall
// time of one test, excluding container startup and compilation.
type Result struct {
	Verdict       string
	PassedTests   int
	TotalTests    int
	FailedTest    int
	TimeMS        int64
	CompileOutput string
}

type Runner interface {
	Run(context.Context, Job) (Result, error)
}

const MaxTrialOutputBytes = 16 * 1024

// TrialResult can expose stdout because its single input was explicitly chosen
// from a public example or supplied by the requesting student.
type TrialResult struct {
	Verdict         string
	TimeMS          int64
	Output          string
	OutputTruncated bool
	CompileOutput   string
}

type TrialRunner interface {
	Trial(ctx context.Context, job Job, compareOutput bool) (TrialResult, error)
}

// ReferenceRunner evaluates the full suite and reports only verdicts and counts.
type ReferenceRunner interface {
	Verify(context.Context, Job) (Result, error)
}

func Supports(language string) bool {
	_, ok := protocol.Language(language)
	return ok
}

func validateJob(job Job) error {
	if !Supports(job.Language) {
		return ErrUnsupportedLanguage
	}
	if len(job.Source) == 0 || len(job.Source) > protocol.MaxSourceBytes {
		return errors.New("source must contain 1–131072 bytes")
	}
	if len(job.Tests) == 0 || len(job.Tests) > protocol.MaxTests {
		return errors.New("automatic checking requires 1–120 tests")
	}
	if job.TimeLimitMS < 100 || job.TimeLimitMS > 30000 || job.MemoryLimitMB < 16 || job.MemoryLimitMB > 2048 {
		return errors.New("unsupported execution limits")
	}
	for _, test := range job.Tests {
		if len(test.Input) > protocol.MaxTestBytes || len(test.Output) > protocol.MaxTestBytes {
			return errors.New("test data exceeds 64 KiB")
		}
	}
	return nil
}
