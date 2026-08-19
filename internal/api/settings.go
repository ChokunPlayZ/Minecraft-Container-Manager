package api

import (
	"net/http"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.DB.QueryContext(r.Context(), `SELECT key, value FROM settings`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read settings")
		return
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not read settings")
			return
		}
		settings[k] = v
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Settings map[string]string `json:"settings"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	tx, err := s.db.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update settings")
		return
	}
	defer tx.Rollback()
	for k, v := range body.Settings {
		if _, err := tx.ExecContext(r.Context(),
			`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			k, v); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not update settings")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": body.Settings})
}
