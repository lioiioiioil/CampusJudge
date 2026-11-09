package app

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/lioiioiioil/CampusJudge/internal/model"
)

func TestSubmissionReadsRespectVisibilityAndOmitSourceFromSummaries(t *testing.T) {
	f := setup(t)
	owner, cookie := f.student("owner")
	other, otherCookie := f.student("other")
	state := f.store.Snapshot()
	work := state.Works[0]
	const source = "package main // private source"
	if err := f.store.Update(func(state *model.State) error {
		hidden := work
		hidden.ID, hidden.Status = "draft-work", "draft"
		state.Works = append(state.Works, hidden)
		state.Submissions = []model.Submission{
			{ID: "visible", UserID: owner.ID, WorkID: work.ID, Source: source, Verdict: "pending", Judging: &model.Judging{Status: "queued"}},
			{ID: "hidden", UserID: owner.ID, WorkID: hidden.ID, Source: source},
			{ID: "foreign", UserID: other.ID, WorkID: work.ID, Source: source},
			{ID: "legacy-empty", UserID: owner.ID, WorkID: work.ID},
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	details := f.request(http.MethodGet, "/api/submissions/visible", nil, cookie, http.StatusOK)
	detail := decode[struct {
		UserID     string           `json:"user_id"`
		Submission model.Submission `json:"submission"`
	}](t, details)
	if detail.UserID != owner.ID || detail.Submission.Source != source {
		t.Fatalf("invalid details: %s", details.Body)
	}
	f.request(http.MethodGet, "/api/submissions/visible", nil, otherCookie, http.StatusNotFound)
	f.request(http.MethodGet, "/api/submissions/hidden", nil, cookie, http.StatusNotFound)
	f.request(http.MethodGet, "/api/submissions/missing", nil, cookie, http.StatusNotFound)
	f.request(http.MethodGet, "/api/submissions/visible", nil, nil, http.StatusUnauthorized)
	f.request(http.MethodGet, "/api/submissions/hidden", nil, f.teacher, http.StatusOK)

	const statuses = "/api/submission-statuses?ids=visible,hidden,foreign,missing,visible"
	f.request(http.MethodGet, statuses, nil, nil, http.StatusUnauthorized)
	statusResponse := f.request(http.MethodGet, statuses, nil, cookie, http.StatusOK)
	summary := decode[struct {
		UserID      string             `json:"user_id"`
		Submissions []model.Submission `json:"submissions"`
	}](t, statusResponse)
	if summary.UserID != owner.ID || len(summary.Submissions) != 1 ||
		summary.Submissions[0].ID != "visible" || summary.Submissions[0].Judging == nil ||
		summary.Submissions[0].Judging.Status != "queued" || strings.Contains(statusResponse.Body.String(), `"source"`) {
		t.Fatalf("invalid status projection: %s", statusResponse.Body)
	}
	teacher := f.request(http.MethodGet, statuses, nil, f.teacher, http.StatusOK)
	if !strings.Contains(teacher.Body.String(), `"id":"hidden"`) || !strings.Contains(teacher.Body.String(), `"id":"foreign"`) ||
		strings.Contains(teacher.Body.String(), `"id":"legacy-empty"`) || strings.Contains(teacher.Body.String(), `"source"`) {
		t.Fatalf("invalid teacher status projection: %s", teacher.Body)
	}
	missing := f.request(http.MethodGet, "/api/submission-statuses?ids=hidden,missing", nil, cookie, http.StatusOK)
	if !strings.Contains(missing.Body.String(), `"submissions":[]`) {
		t.Fatalf("empty status response: %s", missing.Body)
	}

	compact := f.request(http.MethodGet, "/api/state?summary=1", nil, cookie, http.StatusOK)
	compactState := decode[stateResponse](t, compact)
	if compactState.User.ID != owner.ID || len(compactState.Submissions) != 2 ||
		strings.Contains(compact.Body.String(), `"source"`) {
		t.Fatalf("invalid compact state: %s", compact.Body)
	}
	full := f.request(http.MethodGet, "/api/state", nil, cookie, http.StatusOK)
	fullState := decode[stateResponse](t, full)
	if len(fullState.Submissions) != 2 || fullState.Submissions[0].Source != source ||
		!strings.Contains(full.Body.String(), `"source":""`) {
		t.Fatalf("changed full-state contract: %s", full.Body)
	}
	if f.store.Snapshot().Submissions[0].Source != source {
		t.Fatal("summarizing changed persisted source")
	}
}

func TestSubmissionStatusRequestLimit(t *testing.T) {
	f := setup(t)
	const endpoint = "/api/submission-statuses"
	for _, query := range []string{"", "?ids=", "?ids=a,,b", "?ids=a&ids=b", "?ids=" + strings.Repeat("a", 129), "?ids=" + strings.Repeat("id,", 64) + "id"} {
		f.request(http.MethodGet, endpoint+query, nil, f.teacher, http.StatusBadRequest)
	}
	f.request(http.MethodGet, endpoint+"?ids="+url.QueryEscape(strings.Repeat("id,", 63)+"id"), nil, f.teacher, http.StatusOK)
}
