package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"ureach/internal/core"
	"ureach/internal/store"
)

// Follow-ups for quiet leads, in the Campaign tab. John approves every one.
func (a *api) routeNudges(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nudges", a.nudgeQueue)
	mux.HandleFunc("POST /api/nudges/draft", a.nudgeDraft)
	mux.HandleFunc("POST /api/nudges/send", a.nudgeSend)
	mux.HandleFunc("PUT /api/nudges/{id}", a.nudgeDo(func(r *http.Request, id int64) (store.Nudge, error) {
		var body struct{ Script, Followup string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return store.Nudge{}, err
		}
		return a.core.EditNudge(id, body.Script, body.Followup)
	}))
	mux.HandleFunc("POST /api/nudges/{id}/listen", a.nudgeDo(func(r *http.Request, id int64) (store.Nudge, error) {
		return a.core.ListenNudge(r.Context(), id)
	}))
	mux.HandleFunc("POST /api/nudges/{id}/redraft", a.nudgeDo(func(r *http.Request, id int64) (store.Nudge, error) {
		return a.core.Redraft(r.Context(), id)
	}))
	mux.HandleFunc("POST /api/nudges/{id}/skip", a.nudgeDo(func(_ *http.Request, id int64) (store.Nudge, error) {
		if err := a.core.SkipNudge(id); err != nil {
			return store.Nudge{}, err
		}
		return a.store.Nudge(id)
	}))
}

func (a *api) nudgeQueue(w http.ResponseWriter, _ *http.Request) {
	q, err := a.core.Queue()
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, q)
}

// nudgeDraft drafts follow-ups for the quiet leads now, rather than waiting
// for the next automatic pass.
func (a *api) nudgeDraft(w http.ResponseWriter, r *http.Request) {
	// Keep drafting if the browser gives up waiting; the card updates live.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 90*time.Second)
	defer cancel()
	made, err := a.core.DraftNudges(ctx, 6)
	if errors.Is(err, core.ErrDrafting) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "couldn't draft follow-ups: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]int{"drafted": made})
}

func (a *api) nudgeSend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 || len(body.IDs) > 50 {
		http.Error(w, "pick 1 to 50 follow-ups", http.StatusBadRequest)
		return
	}
	ids, err := a.core.SendNudges(body.IDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string][]int64{"sending": ids})
}

// nudgeDo runs an action on one follow-up. Refusals (already sent, the lead
// wrote back) come back as 409 with a sentence the dashboard can show.
func (a *api) nudgeDo(fn func(*http.Request, int64) (store.Nudge, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := pathID(r)
		if id == 0 {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		n, err := fn(r, id)
		if err != nil {
			code := http.StatusConflict
			if strings.HasPrefix(err.Error(), "claude") || strings.HasPrefix(err.Error(), "tts") {
				code = http.StatusBadGateway
			}
			http.Error(w, err.Error(), code)
			return
		}
		writeJSON(w, n)
	}
}
