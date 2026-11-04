package app

import (
	"crypto/rand"
	"errors"
	"os"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
	"github.com/lioiioiioil/CampusJudge/internal/store"
)

type Bootstrap struct {
	Created    bool
	Username   string
	Password   string
	InviteCode string
}

// Initialize creates only the teacher and two explicitly labeled test works.
// Existing teaching data is never overwritten by startup or the demo flag.
func Initialize(s *store.Store, demo bool) (Bootstrap, error) {
	if len(s.Snapshot().Users) > 0 {
		return Bootstrap{}, nil
	}
	password := os.Getenv("CAMPUS_TEACHER_PASSWORD")
	if demo {
		password = "campus-demo"
	}
	if password == "" {
		password = rand.Text()
	}
	if len(password) < 8 || len(password) > 128 {
		return Bootstrap{}, errors.New("CAMPUS_TEACHER_PASSWORD must contain 8–128 bytes")
	}
	code := rand.Text()[:10]
	if demo {
		code = "DEMOCLASS"
	}
	teacher := model.User{ID: newID("user"), Name: "Преподаватель", Username: "teacher", Role: "teacher", PasswordHash: passwordHash(password)}
	group := model.Group{ID: newID("group"), Name: "Тестовая группа", InviteCode: code}
	sum := model.Problem{ID: newID("problem"), Title: "Сумма двух чисел", Statement: "Даны два целых числа a и b. Выведите их сумму.", InputFormat: "Два целых числа a и b, каждое от −1000 до 1000.", OutputFormat: "Одно целое число — сумма a и b.", TimeLimitMS: 2000, MemoryLimitMB: 256, Samples: []model.TestCase{{Input: "2 3\n", Output: "5\n"}}, Tests: []model.TestCase{{Input: "-5 8\n", Output: "3\n"}, {Input: "0 0\n", Output: "0\n"}}}
	largest := model.Problem{ID: newID("problem"), Title: "Большее из двух", Statement: "Даны два целых числа. Выведите наибольшее из них.", InputFormat: "Два целых числа от −1000 до 1000.", OutputFormat: "Наибольшее из двух чисел.", TimeLimitMS: 2000, MemoryLimitMB: 256, Samples: []model.TestCase{{Input: "4 9\n", Output: "9\n"}}, Tests: []model.TestCase{{Input: "-3 -7\n", Output: "-3\n"}, {Input: "5 5\n", Output: "5\n"}}}
	now := time.Now()
	start := now.Add(-time.Minute)
	end := now.Add(24 * time.Hour)
	homework := model.Work{ID: newID("work"), Title: "Тестовая домашка", Description: "Пробное задание для знакомства с платформой. Его можно закрыть, когда подготовите свои материалы.", Kind: "homework", GroupIDs: []string{group.ID}, ProblemIDs: []string{sum.ID}, Status: "open", CreatedAt: now}
	contest := model.Work{ID: newID("work"), Title: "Тестовый контест", Description: "Пробный контест с одной задачей. Решения проверяет преподаватель.", Kind: "contest", GroupIDs: []string{group.ID}, ProblemIDs: []string{largest.ID}, Status: "open", StartsAt: &start, EndsAt: &end, CreatedAt: now}
	if err := s.Update(func(state *model.State) error {
		state.Demo = demo
		state.Users = []model.User{teacher}
		state.Groups = []model.Group{group}
		state.Topics = []model.Topic{}
		state.Problems = []model.Problem{sum, largest}
		state.Works = []model.Work{homework, contest}
		state.Submissions = []model.Submission{}
		state.Sessions = []model.Session{}
		return nil
	}); err != nil {
		return Bootstrap{}, err
	}
	return Bootstrap{Created: true, Username: teacher.Username, Password: password, InviteCode: code}, nil
}
