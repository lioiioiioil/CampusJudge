package app

import (
	"crypto/rand"
	"errors"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

type BootstrapOptions struct {
	Demo            bool
	TeacherPassword string
	Now             time.Time
}

type Bootstrap struct {
	Created    bool
	Username   string
	Password   string
	InviteCode string
}

// Initialize creates a teacher and the two requested test works. Configuration
// and time are explicit inputs; startup never replaces existing teaching data.
func Initialize(s *store.Store, options BootstrapOptions) (Bootstrap, error) {
	if len(s.Snapshot().Users) > 0 {
		return Bootstrap{}, nil
	}
	if options.Now.IsZero() {
		return Bootstrap{}, errors.New("bootstrap time must be provided")
	}
	password := options.TeacherPassword
	if options.Demo {
		password = "campus-demo"
	}
	if password == "" {
		password = rand.Text()
	}
	if len(password) < 8 || len(password) > 128 {
		return Bootstrap{}, errors.New("initial teacher password must contain 8–128 bytes")
	}
	encodedPassword, err := passwordHash(password)
	if err != nil {
		return Bootstrap{}, err
	}
	code := rand.Text()[:10]
	if options.Demo {
		code = "DEMOCLASS"
	}
	teacher := model.User{
		ID:           newID("user"),
		Name:         "Преподаватель",
		Username:     "teacher",
		Role:         "teacher",
		PasswordHash: encodedPassword,
	}
	group := model.Group{ID: newID("group"), Name: "Тестовая группа", InviteCode: code}
	problems := initialProblems()
	start := options.Now.Add(-time.Minute)
	end := options.Now.Add(24 * time.Hour)
	works := []model.Work{
		{
			ID:          newID("work"),
			Title:       "Тестовая домашка",
			Description: "Пробное задание для знакомства с платформой. Его можно закрыть, когда подготовите свои материалы.",
			Kind:        "homework",
			GroupIDs:    []string{group.ID},
			ProblemIDs:  []string{problems[0].ID},
			Status:      "open",
			CreatedAt:   options.Now,
		},
		{
			ID:          newID("work"),
			Title:       "Тестовый контест",
			Description: "Пробный контест с одной задачей. Решения проверяет преподаватель.",
			Kind:        "contest",
			GroupIDs:    []string{group.ID},
			ProblemIDs:  []string{problems[1].ID},
			Status:      "open",
			StartsAt:    &start,
			EndsAt:      &end,
			CreatedAt:   options.Now,
		},
	}
	created := false
	err = s.Update(func(state *model.State) error {
		if len(state.Users) > 0 {
			return nil
		}
		state.Demo = options.Demo
		state.Users = []model.User{teacher}
		state.Groups = []model.Group{group}
		state.Topics = []model.Topic{}
		state.Problems = problems
		state.Works = works
		state.Submissions = []model.Submission{}
		state.Sessions = []model.Session{}
		created = true
		return nil
	})
	if err != nil {
		return Bootstrap{}, err
	}
	if !created {
		return Bootstrap{}, nil
	}
	return Bootstrap{Created: true, Username: teacher.Username, Password: password, InviteCode: code}, nil
}

func initialProblems() []model.Problem {
	return []model.Problem{
		{
			ID:            newID("problem"),
			Title:         "Сумма двух чисел",
			Statement:     "Даны два целых числа a и b. Выведите их сумму.",
			InputFormat:   "Два целых числа a и b, каждое от −1000 до 1000.",
			OutputFormat:  "Одно целое число — сумма a и b.",
			TimeLimitMS:   2000,
			MemoryLimitMB: 256,
			Samples:       []model.TestCase{{Input: "2 3\n", Output: "5\n"}},
			Tests: []model.TestCase{
				{Input: "-5 8\n", Output: "3\n"},
				{Input: "0 0\n", Output: "0\n"},
			},
		},
		{
			ID:            newID("problem"),
			Title:         "Большее из двух",
			Statement:     "Даны два целых числа. Выведите наибольшее из них.",
			InputFormat:   "Два целых числа от −1000 до 1000.",
			OutputFormat:  "Наибольшее из двух чисел.",
			TimeLimitMS:   2000,
			MemoryLimitMB: 256,
			Samples:       []model.TestCase{{Input: "4 9\n", Output: "9\n"}},
			Tests: []model.TestCase{
				{Input: "-3 -7\n", Output: "-3\n"},
				{Input: "5 5\n", Output: "5\n"},
			},
		},
	}
}
