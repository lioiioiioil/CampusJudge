package app

import "time"

const (
	authRateWindow       = time.Minute
	loginAccountBudget   = 6
	loginOriginBudget    = 32
	loginOriginRequests  = 64
	registerOriginBudget = 32
	maxAuthRateEntries   = 10_000
)

type rateEntry struct {
	started time.Time
	count   int
}

type authBudget struct {
	key             string
	limit           int
	refundOnSuccess bool
}

// reserveAuth counts in-flight requests so parallel password guesses cannot
// all pass the limiter before hashing. Successful logins refund failure
// allowances. Total requests and registrations retain their charge.
func (a *App) reserveAuth(budgets ...authBudget) (refund func(), allowed bool) {
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	now := a.now()
	if len(a.attempts)+len(budgets) > maxAuthRateEntries {
		for key, entry := range a.attempts {
			if now.Sub(entry.started) >= authRateWindow {
				delete(a.attempts, key)
			}
		}
	}
	entries := make([]*rateEntry, len(budgets))
	newEntries := 0
	for i, budget := range budgets {
		entry := a.attempts[budget.key]
		if entry == nil {
			newEntries++
		}
		if entry == nil || now.Sub(entry.started) >= authRateWindow {
			entry = &rateEntry{started: now}
		}
		if entry.count >= budget.limit {
			return nil, false
		}
		entries[i] = entry
	}
	if len(a.attempts)+newEntries > maxAuthRateEntries {
		return nil, false
	}
	for i, budget := range budgets {
		entries[i].count++
		a.attempts[budget.key] = entries[i]
	}
	return func() {
		a.rateMu.Lock()
		defer a.rateMu.Unlock()
		for i, budget := range budgets {
			if !budget.refundOnSuccess {
				continue
			}
			entry := entries[i]
			// A late success belongs to its original window, even when another
			// request has already replaced the bucket.
			if a.attempts[budget.key] == entry {
				entry.count--
				if entry.count == 0 {
					delete(a.attempts, budget.key)
				}
			}
		}
	}, true
}
