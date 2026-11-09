package app

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestDraftEditSurvivesPublicationAndRejectsWorksWithSubmissions(t *testing.T) {
	f := setup(t)
	_, student := f.student("draft-editor-student")
	state := f.store.Snapshot()
	groupID := state.Groups[0].ID
	firstID, secondID := state.Problems[0].ID, state.Problems[1].ID
	end := time.Now().Add(24 * time.Hour).UTC()
	draft := decode[model.Work](t, f.request("POST", "/api/works", map[string]any{
		"title": "Черновик", "kind": "homework", "status": "draft", "group_ids": []string{groupID},
		"problem_ids": []string{firstID}, "ends_at": end,
	}, f.teacher, http.StatusCreated))
	workPath := "/api/works/" + draft.ID
	editPath := workPath + "/draft"
	payload := map[string]any{
		"title": "Обновлённая работа", "description": "Порядок задач изменён",
		"group_ids": []string{groupID}, "problem_ids": []string{secondID, firstID},
	}
	f.request("PATCH", editPath, payload, student, http.StatusForbidden)
	bad := map[string]any{"title": "Недопустимая работа", "problem_ids": []string{"missing"}}
	f.request("PATCH", editPath, bad, f.teacher, http.StatusBadRequest)
	updated := decode[model.Work](t, f.request("PATCH", editPath, payload, f.teacher, http.StatusOK))
	if updated.ID != draft.ID || updated.Kind != draft.Kind || updated.Status != "draft" ||
		!updated.CreatedAt.Equal(draft.CreatedAt) || updated.EndsAt != nil || updated.Title != payload["title"] ||
		!slices.Equal(updated.ProblemIDs, []string{secondID, firstID}) {
		t.Fatalf("unexpected draft edit result: %+v", updated)
	}
	if strings.Contains(f.request("GET", "/api/state", nil, student, 200).Body.String(), draft.ID) {
		t.Fatal("editing disclosed the draft")
	}
	f.request("PATCH", workPath, map[string]string{"status": "open"}, f.teacher, http.StatusOK)
	f.request("PATCH", editPath, payload, f.teacher, http.StatusConflict)
	f.request("POST", "/api/submissions", map[string]string{
		"work_id": draft.ID, "problem_id": secondID, "language": "python", "source": "print(1)",
	}, student, http.StatusCreated)
	f.request("PATCH", workPath, map[string]string{"status": "draft"}, f.teacher, http.StatusOK)
	f.request("PATCH", editPath, payload, f.teacher, http.StatusConflict)
}
