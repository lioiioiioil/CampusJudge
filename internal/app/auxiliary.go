package app

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/judge"
	"github.com/lioiioiioil/CampusJudge/internal/judge/protocol"
	"github.com/lioiioiioil/CampusJudge/internal/model"
)

const (
	auxiliaryResultLimit = 64
	auxiliaryResultTTL   = 5 * time.Minute
	auxiliaryCooldown    = 5 * time.Second
)

type auxiliarySource struct {
	ActorID  string `json:"actor_id"`
	Language string `json:"language"`
	Source   string `json:"source"`
}

func (source auxiliarySource) validate(actor model.User) error {
	if source.ActorID != actor.ID {
		return fail(http.StatusConflict, "Аккаунт изменился. Обновите страницу")
	}
	if len(source.Source) > protocol.MaxSourceBytes || strings.TrimSpace(source.Source) == "" {
		return fail(http.StatusBadRequest, "Укажите исходный код размером до 128 КБ")
	}
	if !judge.Supports(source.Language) {
		return fail(http.StatusBadRequest, "Для выбранного языка автоматический запуск недоступен")
	}
	return nil
}

type auxiliaryResult struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	Verdict         string `json:"verdict,omitempty"`
	TimeMS          int64  `json:"time_ms"`
	Output          string `json:"output,omitempty"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	CompileOutput   string `json:"compile_output,omitempty"`
}

type cachedAuxiliaryRun struct {
	ownerID   string
	startedAt time.Time
	expiresAt time.Time
	result    auxiliaryResult
}

// The slot is shared with the durable judge. Auxiliary runs are admitted only
// when it is free, so they cannot pile up or double the executor's memory load.
type auxiliaryRuns struct {
	ctx     context.Context
	slot    chan struct{}
	mu      sync.Mutex
	stopped bool
	tasks   sync.WaitGroup
	results map[string]cachedAuxiliaryRun
}

func newAuxiliaryRuns(ctx context.Context) auxiliaryRuns {
	return auxiliaryRuns{ctx: ctx, slot: make(chan struct{}, 1), results: make(map[string]cachedAuxiliaryRun)}
}

func (runs *auxiliaryRuns) wait() {
	runs.mu.Lock()
	runs.stopped = true
	runs.mu.Unlock()
	runs.tasks.Wait()
}

func (a *App) runGraded(ctx context.Context, job judge.Job) (judge.Result, error) {
	select {
	case <-ctx.Done():
		return judge.Result{}, ctx.Err()
	case a.auxiliary.slot <- struct{}{}:
	}
	defer func() { <-a.auxiliary.slot }()
	if err := ctx.Err(); err != nil {
		return judge.Result{}, err
	}
	return a.runner.Run(ctx, job)
}

func (a *App) startAuxiliary(user model.User, submissions []model.Submission, deadline time.Duration, execute func(context.Context) (auxiliaryResult, error)) (auxiliaryResult, error) {
	runs := &a.auxiliary
	runs.mu.Lock()
	defer runs.mu.Unlock()
	if runs.ctx == nil || runs.stopped || runs.ctx.Err() != nil {
		return auxiliaryResult{}, fail(http.StatusServiceUnavailable, "Исполнитель сейчас недоступен")
	}
	if slices.ContainsFunc(submissions, automaticPending) {
		return auxiliaryResult{}, fail(http.StatusTooManyRequests, "Исполнитель проверяет посылки. Попробуйте чуть позже")
	}
	now := a.now()
	var oldestID string
	var oldest time.Time
	for id, entry := range runs.results {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			delete(runs.results, id)
			continue
		}
		if entry.ownerID == user.ID && now.Sub(entry.startedAt) < auxiliaryCooldown {
			return auxiliaryResult{}, fail(http.StatusTooManyRequests, "Подождите несколько секунд перед следующим запуском")
		}
		if !entry.expiresAt.IsZero() && (oldestID == "" || entry.expiresAt.Before(oldest)) {
			oldestID, oldest = id, entry.expiresAt
		}
	}
	select {
	case runs.slot <- struct{}{}:
	default:
		return auxiliaryResult{}, fail(http.StatusTooManyRequests, "Исполнитель занят. Попробуйте чуть позже")
	}
	if len(runs.results) >= auxiliaryResultLimit {
		delete(runs.results, oldestID)
	}
	result := auxiliaryResult{ID: newID("run"), Status: "running"}
	entry := cachedAuxiliaryRun{ownerID: user.ID, startedAt: now, result: result}
	runs.results[result.ID] = entry
	runs.tasks.Add(1)
	go func() {
		defer runs.tasks.Done()
		ctx, cancel := context.WithTimeout(runs.ctx, deadline)
		finished, err := execute(ctx)
		timedOut := ctx.Err() == context.DeadlineExceeded
		cancel()
		if err != nil || timedOut {
			slog.Error("auxiliary execution failed", "run", result.ID, "error", err)
			message := "Исполнитель не смог завершить запуск. Попробуйте позже"
			if timedOut {
				message = "Достигнут общий лимит времени запуска"
			}
			finished = auxiliaryResult{Status: "error", Error: message}
		} else {
			finished.Status = "done"
		}
		finished.ID = result.ID
		runs.mu.Lock()
		entry.result, entry.expiresAt = finished, a.now().Add(auxiliaryResultTTL)
		runs.results[result.ID] = entry
		<-runs.slot
		runs.mu.Unlock()
	}()
	return result, nil
}

func (a *App) auxiliaryResult(w http.ResponseWriter, r *http.Request, user model.User) error {
	runs := &a.auxiliary
	runs.mu.Lock()
	entry, found := runs.results[r.PathValue("id")]
	runs.mu.Unlock()
	if !found || entry.ownerID != user.ID || (!entry.expiresAt.IsZero() && !a.now().Before(entry.expiresAt)) {
		return fail(http.StatusNotFound, "Результат запуска не найден или срок его хранения истёк")
	}
	writeJSON(w, http.StatusOK, entry.result)
	return nil
}
