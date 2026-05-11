package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/lyzrai/flow/pkg/secrets"
	"github.com/lyzrai/flow/pkg/storage"
)

// Global (org-wide) credentials. Per-agent overrides live on
// agent.Credentials and are handled by credentials.go; this file owns
// the shared pool. Lookup precedence at runtime (Deploy executor):
//
//	node.parameters.credentialName
//	  → agent.Credentials[name]   (agent-specific override)
//	  → credentials[name]         (this global pool)
//
// API surface:
//   GET    /api/credentials
//   POST   /api/credentials
//   GET    /api/credentials/{name}
//   PUT    /api/credentials/{name}
//   DELETE /api/credentials/{name}

func (s *Server) handleListGlobalCredentials(w http.ResponseWriter, r *http.Request) {
	if s.credentials == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credentials store not configured"))
		return
	}
	list, err := s.credentials.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]PublicCredential, 0, len(list))
	for _, c := range list {
		out = append(out, publicCredential(*c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetGlobalCredential(w http.ResponseWriter, r *http.Request) {
	if s.credentials == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credentials store not configured"))
		return
	}
	name := r.PathValue("name")
	c, err := s.credentials.GetByName(r.Context(), name)
	if err != nil {
		writeStorageErr(w, err, "credential not found")
		return
	}
	writeJSON(w, http.StatusOK, publicCredential(*c))
}

func (s *Server) handleCreateGlobalCredential(w http.ResponseWriter, r *http.Request) {
	if !secrets.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, errors.New("FLOW_SECRET_KEY is not set; refusing to store credentials"))
		return
	}
	if s.credentials == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credentials store not configured"))
		return
	}
	var body credentialBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cred, err := body.validateAndBuild()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cred.ID = newID()
	if err := s.credentials.Create(r.Context(), &cred); err != nil {
		if errors.Is(err, storage.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, publicCredential(cred))
}

func (s *Server) handleUpdateGlobalCredential(w http.ResponseWriter, r *http.Request) {
	if !secrets.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, errors.New("FLOW_SECRET_KEY is not set"))
		return
	}
	if s.credentials == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credentials store not configured"))
		return
	}
	name := r.PathValue("name")
	existing, err := s.credentials.GetByName(r.Context(), name)
	if err != nil {
		writeStorageErr(w, err, "credential not found")
		return
	}

	var body credentialBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Name == "" {
		body.Name = name
	}
	updated, err := body.validateAndBuild()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Renaming would require a delete+create against the unique index;
	// keep the name immutable through PUT and force the user to recreate
	// to rename. Mirrors per-agent behavior.
	updated.Name = existing.Name
	updated.ID = existing.ID
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()

	// Preserve unchanged secrets when the request omits them.
	if updated.Type == storage.CredentialGCP && updated.GcpServiceAccountSealed == "" {
		updated.GcpServiceAccountSealed = existing.GcpServiceAccountSealed
	}
	if updated.Type == storage.CredentialKV && len(updated.KvSealed) == 0 {
		updated.KvSealed = existing.KvSealed
	}

	if err := s.credentials.Update(r.Context(), &updated); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, publicCredential(updated))
}

func (s *Server) handleDeleteGlobalCredential(w http.ResponseWriter, r *http.Request) {
	if s.credentials == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("credentials store not configured"))
		return
	}
	name := r.PathValue("name")
	if err := s.credentials.Delete(r.Context(), name); err != nil {
		writeStorageErr(w, err, "credential not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
