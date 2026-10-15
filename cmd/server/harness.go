package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"ureach/internal/brain"
	"ureach/internal/harness"
	"ureach/internal/store"
)

// John's harness: presets, persona and reply style, FAQ, referral link and
// the uploaded sales playbook. Every change applies to the next reply.
func (a *api) routeHarness(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/harness", a.harnessGet)
	mux.HandleFunc("PUT /api/harness/active", a.harnessSaveActive)
	mux.HandleFunc("POST /api/harness/presets", a.harnessSaveAs)
	mux.HandleFunc("POST /api/harness/presets/{id}/activate", a.harnessDo(func(id int64, _ *http.Request) error { return a.harness.Activate(id) }))
	mux.HandleFunc("POST /api/harness/presets/{id}/reset", a.harnessDo(func(id int64, _ *http.Request) error { return a.harness.Reset(id) }))
	mux.HandleFunc("DELETE /api/harness/presets/{id}", a.harnessDo(func(id int64, _ *http.Request) error { return a.harness.Delete(id) }))
	mux.HandleFunc("PATCH /api/harness/presets/{id}", a.harnessDo(func(id int64, r *http.Request) error {
		var body struct{ Name, Description string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return err
		}
		return a.harness.Rename(id, body.Name, body.Description)
	}))
	mux.HandleFunc("PUT /api/harness/faq", a.harnessDo(func(_ int64, r *http.Request) error {
		var body struct{ Text string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return err
		}
		return a.harness.SetFAQ(body.Text)
	}))
	mux.HandleFunc("PUT /api/harness/referral", a.harnessDo(func(_ int64, r *http.Request) error {
		var body struct{ URL string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return err
		}
		return a.harness.SetReferral(body.URL)
	}))
	mux.HandleFunc("POST /api/harness/documents", a.harnessUpload)
	mux.HandleFunc("GET /api/harness/documents/{id}", a.harnessDocText)
	mux.HandleFunc("PATCH /api/harness/documents/{id}", a.harnessDo(func(id int64, r *http.Request) error {
		var body struct{ Active bool }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return err
		}
		return a.harness.SetDocumentActive(id, body.Active)
	}))
	mux.HandleFunc("DELETE /api/harness/documents/{id}", a.harnessDo(func(id int64, _ *http.Request) error { return a.harness.DeleteDocument(id) }))
	mux.HandleFunc("POST /api/harness/try", a.harnessTry)
}

func (a *api) harnessGet(w http.ResponseWriter, r *http.Request) {
	snap, err := a.harness.Snapshot()
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, snap)
}

// harnessDo runs a change, tells every open dashboard, and returns the new harness.
func (a *api) harnessDo(fn func(id int64, r *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(pathID(r), r); err != nil {
			harnessErr(w, err)
			return
		}
		a.harnessChanged(w)
	}
}

func (a *api) harnessChanged(w http.ResponseWriter) {
	a.core.Hub.Publish("harness", nil)
	a.harnessGet(w, nil)
}

func (a *api) harnessSaveActive(w http.ResponseWriter, r *http.Request) {
	var h harness.Harness
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil {
		harnessErr(w, err)
		return
	}
	if err := a.harness.SaveActive(h); err != nil {
		harnessErr(w, err)
		return
	}
	a.harnessChanged(w)
}

func (a *api) harnessSaveAs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Harness     harness.Harness `json:"harness"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		harnessErr(w, err)
		return
	}
	if _, err := a.harness.SaveAs(body.Name, body.Description, body.Harness); err != nil {
		harnessErr(w, err)
		return
	}
	a.harnessChanged(w)
}

// harnessUpload adds a .md, .txt or .pdf to John's sales playbook.
func (a *api) harnessUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "upload a file up to 15 MB", http.StatusBadRequest)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "upload a file up to 15 MB", http.StatusBadRequest)
		return
	}
	if _, err := a.harness.AddDocument(hdr.Filename, data); err != nil {
		harnessErr(w, err)
		return
	}
	a.harnessChanged(w)
}

func (a *api) harnessDocText(w http.ResponseWriter, r *http.Request) {
	text, err := a.harness.DocumentText(pathID(r))
	if err != nil {
		harnessErr(w, err)
		return
	}
	writeJSON(w, map[string]string{"text": text})
}

// harnessTry writes John's reply to a pretend message with the current
// harness. Nothing is saved or sent.
func (a *api) harnessTry(w http.ResponseWriter, r *http.Request) {
	var body struct{ Name, Message string }
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Message) == "" {
		http.Error(w, "type a message to try", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	reply, meta, err := a.brain.Reply(ctx, brain.Contact{Name: strings.TrimSpace(body.Name)}, []brain.Turn{{FromLead: true, Text: body.Message}})
	if err != nil {
		http.Error(w, "John's AI couldn't answer: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]any{
		"reply":          reply,
		"words":          meta.Words,
		"trimmed":        meta.Trimmed,
		"forced_handoff": meta.ForcedHandoff,
		"model":          meta.Model,
		"latency_ms":     meta.Latency.Milliseconds(),
	})
}

func harnessErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, harness.ErrInvalid):
		http.Error(w, strings.TrimPrefix(err.Error(), harness.ErrInvalid.Error()+": "), http.StatusBadRequest)
	case errors.Is(err, store.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
			return
		}
		httpErr(w, err)
	}
}
