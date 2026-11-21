package server

import (
	"errors"
	"net/http"

	"harness.engineering/internal/model"
)

func (app server) setProjectColor(w http.ResponseWriter, r *http.Request) {
	id, err := projectID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid project color request", http.StatusBadRequest)
		return
	}
	if err := app.store.SetProjectColor(r.Context(), id, r.PostForm.Get("preset")); err != nil {
		var invalid model.Invalid
		if errors.As(err, &invalid) {
			http.Error(w, invalid.Error(), http.StatusBadRequest)
			return
		}
		pageError(w, r, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
