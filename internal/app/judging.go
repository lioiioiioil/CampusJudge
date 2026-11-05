package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

// StartJudge is called once before serving HTTP. A single consumer keeps the
// resource budget predictable; accepted jobs live in the same durable JSON as
// submissions, so a lost wake-up or restart cannot lose the queued work.
func (a *App) StartJudge(ctx context.Context, runner judge.Runner) error {
	if runner == nil || a.runner != nil {
		return errors.New("judge must be configured once with a runner")
	}
	if err := a.prepareJudgeQueue(true); err != nil {
		return err
	}
	a.runner = runner
	a.judgeWake = make(chan struct{}, 1)
	a.judgeDone = make(chan struct{})
	go a.judgeLoop(ctx)
	a.wakeJudge()
	return nil
}

// Manual installations may already contain pending solutions when automatic
// judging is enabled. Admit those in stored arrival order, respecting the same
// bounds as new submissions; completed teacher decisions are never adopted.
func prepareQueue(state *model.State, recoverRunning bool) bool {
	changed, active := false, 0
	byUser := make(map[string]int)
	for i := range state.Submissions {
		sub := &state.Submissions[i]
		if automaticPending(*sub) {
			active++
			byUser[sub.UserID]++
			if recoverRunning && sub.Judging.Status == "running" {
				sub.Judging.Status = "queued"
				changed = true
			}
		}
	}
	for i := range state.Submissions {
		sub := &state.Submissions[i]
		if active >= 64 {
			break
		}
		if sub.Verdict != "pending" || sub.ReviewerID != "" || sub.Judging != nil ||
			!automaticLanguage(sub.Language) || byUser[sub.UserID] >= 3 ||
			len(sub.Source) > 128<<10 || strings.TrimSpace(sub.Source) == "" {
			continue
		}
		problem := problemByID(*state, sub.ProblemID)
		total := len(problem.Samples) + len(problem.Tests)
		if problem.ID == "" || total == 0 {
			continue
		}
		sub.Judging = &model.Judging{Status: "queued", TotalTests: total}
		active++
		byUser[sub.UserID]++
		changed = true
	}
	return changed
}

func (a *App) prepareJudgeQueue(recoverRunning bool) error {
	probe := a.store.Snapshot()
	if !prepareQueue(&probe, recoverRunning) {
		return nil
	}
	return a.store.Update(func(state *model.State) error { prepareQueue(state, recoverRunning); return nil })
}

func (a *App) WaitJudge() {
	if a.judgeDone != nil {
		<-a.judgeDone
	}
}

func (a *App) wakeJudge() {
	if a.judgeWake == nil {
		return
	}
	select {
	case a.judgeWake <- struct{}{}:
	default:
	}
}

func automaticPending(sub model.Submission) bool {
	return sub.Verdict == "pending" && sub.ReviewerID == "" && sub.Judging != nil &&
		(sub.Judging.Status == "queued" || sub.Judging.Status == "running")
}

func problemByID(s model.State, id string) model.Problem {
	for _, problem := range s.Problems {
		if problem.ID == id {
			return problem
		}
	}
	return model.Problem{}
}

func (a *App) judgeLoop(ctx context.Context) {
	defer close(a.judgeDone)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.judgeWake:
		case <-tick.C:
		}
		for ctx.Err() == nil {
			// Reconcile between jobs, not only on an idle tick: a continuously
			// occupied queue must still admit earlier pending upgrade work.
			if err := a.prepareJudgeQueue(false); err != nil {
				slog.Error("judge queue preparation failed", "error", err)
				break
			}
			var next *model.Submission
			for _, sub := range a.store.Snapshot().Submissions {
				if automaticPending(sub) {
					copy := sub
					next = &copy
					break
				}
			}
			if next == nil {
				break
			}
			if err := a.judgeOne(ctx, next.ID); err != nil {
				if ctx.Err() == nil {
					slog.Error("judge transaction failed", "submission", next.ID, "error", err)
				}
				break // Reconcile from disk after the next tick, not in a busy loop.
			}
		}
	}
}

func (a *App) judgeOne(ctx context.Context, id string) error {
	var job judge.Job
	claimed := false
	if err := a.store.Update(func(s *model.State) error {
		for i := range s.Submissions {
			sub := &s.Submissions[i]
			if sub.ID != id || !automaticPending(*sub) {
				continue
			}
			problem := problemByID(*s, sub.ProblemID)
			job = judge.Job{Language: sub.Language, Source: sub.Source,
				Tests:       append(slices.Clone(problem.Samples), problem.Tests...),
				TimeLimitMS: problem.TimeLimitMS, MemoryLimitMB: problem.MemoryLimitMB}
			sub.Judging.Status = "running"
			claimed = true
			break
		}
		return nil
	}); err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	result, runErr := a.runner.Run(ctx, job)
	if ctx.Err() != nil {
		return ctx.Err()
	} // Persisted running job is resumed next startup.
	if runErr == nil && (!slices.Contains([]string{"accepted", "wrong_answer", "compilation_error", "runtime_error", "time_limit", "memory_limit", "output_limit"}, result.Verdict) ||
		result.TotalTests != len(job.Tests) || result.TotalTests == 0 || result.PassedTests < 0 || result.PassedTests > result.TotalTests ||
		result.TimeMS < 0 || (result.Verdict == "accepted" && result.PassedTests != result.TotalTests)) {
		runErr = errors.New("runner returned an inconsistent result")
	}
	if runErr != nil {
		slog.Error("judge execution failed", "submission", id, "error", runErr)
	}
	// A storage failure must not execute the student's program again: keep the
	// obtained result and retry its conditional commit until shutdown.
	for {
		err := a.store.Update(func(s *model.State) error {
			for i := range s.Submissions {
				sub := &s.Submissions[i]
				if sub.ID != id || !automaticPending(*sub) || sub.Judging.Status != "running" {
					continue
				}
				now := a.now()
				sub.JudgedAt = &now
				if runErr != nil {
					sub.Verdict = "judge_error"
					sub.Judging.Status = "error"
					sub.Judging.Error = "Автоматическая проверка временно недоступна. Преподаватель может проверить решение вручную."
				} else {
					sub.Verdict = result.Verdict
					sub.Judging = &model.Judging{Status: "done", PassedTests: result.PassedTests,
						TotalTests: result.TotalTests, FailedTest: result.FailedTest, TimeMS: result.TimeMS,
						CompileOutput: result.CompileOutput}
				}
				break
			}
			return nil
		})
		if err == nil {
			return nil
		}
		slog.Error("judge result save failed; retrying", "submission", id, "error", err)
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// Explicit manual mode must not advertise old automatic jobs as still running.
// They remain available for review and do not contribute contest penalties.
func (a *App) UseManualJudging() error {
	needed := false
	for _, sub := range a.store.Snapshot().Submissions {
		if automaticPending(sub) {
			needed = true
			break
		}
	}
	if !needed {
		return nil
	}
	return a.store.Update(func(s *model.State) error {
		for i := range s.Submissions {
			if automaticPending(s.Submissions[i]) {
				sub := &s.Submissions[i]
				sub.Verdict = "judge_error"
				sub.Judging.Status = "error"
				sub.Judging.Error = "Автоматическая проверка выключена. Решение доступно для ручной проверки."
			}
		}
		return nil
	})
}
