// Package domain owns the classroom rules independently of HTTP and persistence.
package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

var (
	ErrNotFound    = errors.New("Работа не найдена")
	ErrStudentOnly = errors.New("Отправка решений доступна из аккаунта ученика")
	ErrClosed      = errors.New("Приём решений закрыт или ещё не начался")
	ErrProblem     = errors.New("В этой работе нет выбранной задачи")
)

// WorkVisible controls access to a work's card, including closed or upcoming
// work. Access to its problems additionally depends on WorkStarted.
func WorkVisible(work model.Work, user model.User) bool {
	return user.Role == "teacher" || (work.Status != "draft" && slices.Contains(work.GroupIDs, user.GroupID))
}

func WorkStarted(work model.Work, now time.Time) bool {
	return work.StartsAt == nil || !now.Before(*work.StartsAt)
}

// CanSubmit checks admission at the instant supplied by the caller. StartsAt is
// inclusive and EndsAt exclusive. Inaccessible work is indistinguishable from
// missing work, so its schedule and problems are not disclosed.
func CanSubmit(work model.Work, user model.User, problemID string, now time.Time) error {
	if user.Role != "student" {
		return ErrStudentOnly
	}
	if work.ID == "" || !WorkVisible(work, user) {
		return ErrNotFound
	}
	if work.Status != "open" || !WorkStarted(work, now) || (work.EndsAt != nil && !now.Before(*work.EndsAt)) {
		return ErrClosed
	}
	if !slices.Contains(work.ProblemIDs, problemID) {
		return ErrProblem
	}
	return nil
}

// CanEditDraft protects the structure of any work with existing submissions,
// including a previously published work returned to draft.
func CanEditDraft(work model.Work, submissions []model.Submission) bool {
	return work.Status == "draft" && !slices.ContainsFunc(submissions, func(submission model.Submission) bool {
		return submission.WorkID == work.ID
	})
}

func ValidWorkStatus(status string) bool {
	return status == "draft" || status == "open" || status == "closed"
}

// ValidateWork validates the complete prospective work against the same state
// that will receive it. Callers apply input defaults before validation and run
// this check inside their write transaction when referenced data may change.
func ValidateWork(state model.State, work model.Work) error {
	title := strings.TrimSpace(work.Title)
	if len(title) < 2 || len(title) > 200 || len(work.Description) > 10000 {
		return errors.New("Укажите название работы и описание допустимой длины")
	}
	if work.Kind != "homework" && work.Kind != "contest" {
		return errors.New("Тип работы: homework или contest")
	}
	if !ValidWorkStatus(work.Status) {
		return errors.New("Неизвестный статус работы")
	}
	if work.StartsAt != nil && work.EndsAt != nil && !work.EndsAt.After(*work.StartsAt) {
		return errors.New("Окончание должно быть позже начала")
	}
	if work.Kind == "contest" && (work.StartsAt == nil || work.EndsAt == nil) {
		return errors.New("Для контеста укажите начало и окончание")
	}
	if len(work.ProblemIDs) > 50 || len(work.GroupIDs) > 100 {
		return errors.New("Слишком много задач или групп")
	}
	if work.TopicID != "" && !slices.ContainsFunc(state.Topics, func(topic model.Topic) bool { return topic.ID == work.TopicID }) {
		return errors.New("Тема не найдена")
	}
	seen := make(map[string]bool)
	for _, id := range work.GroupIDs {
		if seen[id] {
			return errors.New("Группа указана дважды")
		}
		seen[id] = true
		if !slices.ContainsFunc(state.Groups, func(group model.Group) bool { return group.ID == id }) {
			return errors.New("Группа не найдена")
		}
	}
	seen = make(map[string]bool)
	for _, id := range work.ProblemIDs {
		if seen[id] {
			return errors.New("Задача указана дважды")
		}
		seen[id] = true
		if !slices.ContainsFunc(state.Problems, func(problem model.Problem) bool { return problem.ID == id }) {
			return errors.New("Задача не найдена")
		}
	}
	if work.Status != "draft" && (len(work.GroupIDs) == 0 || len(work.ProblemIDs) == 0) {
		return errors.New("Для публикации выберите хотя бы одну группу и задачу")
	}
	return nil
}
