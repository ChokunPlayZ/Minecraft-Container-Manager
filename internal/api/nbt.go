package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mcm-panel/mcm/internal/nbt"
	"github.com/mcm-panel/mcm/internal/servers"
)

func (s *Server) writeNBTErr(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, servers.ErrNBTBusy):
		writeError(w, 409, "nbt_busy", err.Error())
	case errors.Is(err, servers.ErrNBTChanged):
		writeError(w, 409, "nbt_changed", err.Error())
	case errors.Is(err, servers.ErrNotFound):
		s.writeServerErr(w, err)
	case errors.Is(err, nbt.ErrInvalid):
		writeError(w, 400, "invalid_nbt", err.Error())
	default:
		s.writeFileErr(w, r, err)
	}
}
func (s *Server) handleReadNBT(w http.ResponseWriter, r *http.Request) {
	result, err := s.servers.ReadNBT(r.Context(), r.PathValue("id"), r.URL.Query().Get("path"))
	if err != nil {
		s.writeNBTErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, result)
}
func (s *Server) handleWriteNBT(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path             string  `json:"path"`
		ExpectedRevision string  `json:"expected_revision"`
		Root             nbt.Tag `json:"root"`
	}
	// String-valued arrays and typed tree nodes expand beyond their binary size.
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeError(w, 400, "invalid_request", "Invalid NBT document or request exceeds 64 MiB.")
		return
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		writeError(w, 400, "invalid_request", "Send one NBT document.")
		return
	}
	if in.Path == "" || in.ExpectedRevision == "" {
		writeError(w, 400, "invalid_request", "A path and the opened file revision are required.")
		return
	}
	result, err := s.servers.WriteNBT(r.Context(), r.PathValue("id"), in.Path, in.ExpectedRevision, nbt.Document{Root: in.Root})
	if err != nil {
		s.writeNBTErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, result)
}
func (s *Server) handleListPlayerData(w http.ResponseWriter, r *http.Request) {
	players, err := s.servers.ListPlayerData(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeNBTErr(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"players": players})
}
