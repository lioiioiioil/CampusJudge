package model

import "time"

type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Username     string `json:"username"`
	Role         string `json:"role"`
	GroupID      string `json:"group_id,omitempty"`
	PasswordHash string `json:"password_hash"`
}

type PublicUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Role     string `json:"role"`
	GroupID  string `json:"group_id,omitempty"`
}

func (u User) Public() PublicUser {
	return PublicUser{
		ID:       u.ID,
		Name:     u.Name,
		Username: u.Username,
		Role:     u.Role,
		GroupID:  u.GroupID,
	}
}

type Group struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	InviteCode string `json:"invite_code,omitempty"`
}
type Topic struct {
	LessonURL   string `json:"lesson_url,omitempty"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}
type TestCase struct {
	Input  string `json:"input"`
	Output string `json:"output"`
}
type TestRevision struct {
	Number    int        `json:"number"`
	Samples   []TestCase `json:"samples"`
	Tests     []TestCase `json:"tests,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
type Problem struct {
	ID            string         `json:"id"`
	Title         string         `json:"title"`
	Statement     string         `json:"statement"`
	InputFormat   string         `json:"input_format"`
	OutputFormat  string         `json:"output_format"`
	TimeLimitMS   int            `json:"time_limit_ms"`
	MemoryLimitMB int            `json:"memory_limit_mb"`
	Samples       []TestCase     `json:"samples"`
	Tests         []TestCase     `json:"tests,omitempty"`
	TestRevisions []TestRevision `json:"test_revisions,omitempty"`
}
type Work struct {
	ID               string         `json:"id"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	Kind             string         `json:"kind"`
	TopicID          string         `json:"topic_id"`
	GroupIDs         []string       `json:"group_ids"`
	ProblemIDs       []string       `json:"problem_ids"`
	ProblemRevisions map[string]int `json:"problem_revisions,omitempty"`
	Status           string         `json:"status"`
	StartsAt         *time.Time     `json:"starts_at,omitempty"`
	EndsAt           *time.Time     `json:"ends_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}
type Submission struct {
	ID           string     `json:"id"`
	WorkID       string     `json:"work_id"`
	ProblemID    string     `json:"problem_id"`
	UserID       string     `json:"user_id"`
	Language     string     `json:"language"`
	Source       string     `json:"source"`
	Verdict      string     `json:"verdict"`
	Feedback     string     `json:"feedback"`
	CreatedAt    time.Time  `json:"created_at"`
	JudgedAt     *time.Time `json:"judged_at,omitempty"`
	ReviewerID   string     `json:"reviewer_id,omitempty"`
	Judging      *Judging   `json:"judging,omitempty"`
	TestRevision int        `json:"test_revision,omitempty"`
}

// Judging contains only public progress. Hidden test inputs/answers never become
// part of a submission or leave the worker's trusted comparison boundary.
type Judging struct {
	Status        string `json:"status"`
	PassedTests   int    `json:"passed_tests"`
	TotalTests    int    `json:"total_tests"`
	FailedTest    int    `json:"failed_test,omitempty"`
	TimeMS        int64  `json:"time_ms"`
	CompileOutput string `json:"compile_output,omitempty"`
	Error         string `json:"error,omitempty"`
}
type Session struct {
	TokenHash string    `json:"token_hash"`
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
}
type State struct {
	Demo        bool         `json:"demo"`
	Version     int          `json:"version"`
	Users       []User       `json:"users"`
	Groups      []Group      `json:"groups"`
	Topics      []Topic      `json:"topics"`
	Problems    []Problem    `json:"problems"`
	Works       []Work       `json:"works"`
	Submissions []Submission `json:"submissions"`
	Sessions    []Session    `json:"sessions"`
}
