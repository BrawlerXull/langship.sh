package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lyzrai/flow/pkg/storage"
)

// Environments API — global, named deploy stages. An environment is a
// sequencing container: it owns an ordered list of pipelines (the
// promotion sequence) plus a description. Per-deploy config lives on the
// nodes, not here. Agents subscribe to environments (see agents.go);
// dispatch fans out across the followed envs' pipelines.
//
// Routes:
//   GET    /api/environments
//   POST   /api/environments
//   GET    /api/environments/{name}
//   PUT    /api/environments/{name}                          (name + description)
//   DELETE /api/environments/{name}
//   POST   /api/environments/{name}/pipelines/{pipelineId}   (bring a pipeline in — appends)
//   DELETE /api/environments/{name}/pipelines/{pipelineId}   (remove a pipeline)
//   PUT    /api/environments/{name}/pipelines                (reorder; body {"pipelineIds":[...]})

// environmentBody is the create/update wire shape. PipelineIDs are
// managed via the /pipelines sub-routes.
type environmentBody struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (b *environmentBody) normalize() (storage.Environment, error) {
	name := strings.TrimSpace(b.Name)
	if name == "" {
		return storage.Environment{}, errors.New("name is required")
	}
	now := time.Now().UTC()
	return storage.Environment{
		Name:        name,
		Description: strings.TrimSpace(b.Description),
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	list, err := s.environments.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if list == nil {
		list = []*storage.Environment{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetEnvironment(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	e, err := s.environments.GetByName(r.Context(), r.PathValue("name"))
	if err != nil {
		writeStorageErr(w, err, "environment not found")
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) handleCreateEnvironment(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	var body environmentBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env, err := body.normalize()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	env.ID = newID()
	if err := s.environments.Create(r.Context(), &env); err != nil {
		if errors.Is(err, storage.ErrAlreadyExists) {
			writeError(w, http.StatusConflict, fmt.Errorf("environment %q already exists", env.Name))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, &env)
}

func (s *Server) handleUpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	name := r.PathValue("name")
	existing, err := s.environments.GetByName(r.Context(), name)
	if err != nil {
		writeStorageErr(w, err, "environment not found")
		return
	}
	var body environmentBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Name == "" {
		body.Name = name
	}
	updated, err := body.normalize()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Name is immutable through PUT; carry forward identity + pipeline set.
	updated.Name = existing.Name
	updated.ID = existing.ID
	updated.PipelineIDs = existing.PipelineIDs
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	if err := s.environments.Update(r.Context(), &updated); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, &updated)
}

func (s *Server) handleDeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	if err := s.environments.Delete(r.Context(), r.PathValue("name")); err != nil {
		writeStorageErr(w, err, "environment not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEnvAddPipeline(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	name := r.PathValue("name")
	pipelineID := r.PathValue("pipelineId")
	env, err := s.environments.GetByName(r.Context(), name)
	if err != nil {
		writeStorageErr(w, err, "environment not found")
		return
	}
	if _, err := s.pipelines.Get(r.Context(), pipelineID); err != nil {
		writeStorageErr(w, err, "pipeline not found")
		return
	}
	if env.HasPipeline(pipelineID) {
		writeJSON(w, http.StatusOK, env) // already in this env
		return
	}
	env.PipelineIDs = append(env.PipelineIDs, pipelineID)
	env.UpdatedAt = time.Now().UTC()
	if err := s.environments.Update(r.Context(), env); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) handleEnvRemovePipeline(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	name := r.PathValue("name")
	pipelineID := r.PathValue("pipelineId")
	env, err := s.environments.GetByName(r.Context(), name)
	if err != nil {
		writeStorageErr(w, err, "environment not found")
		return
	}
	out := env.PipelineIDs[:0]
	for _, p := range env.PipelineIDs {
		if p != pipelineID {
			out = append(out, p)
		}
	}
	env.PipelineIDs = out
	env.UpdatedAt = time.Now().UTC()
	if err := s.environments.Update(r.Context(), env); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEnvReorderPipelines replaces the env's pipeline order. The body
// must be a permutation of the env's current pipeline set — extra or
// missing IDs are rejected so a stale client can't silently drop a
// pipeline.
func (s *Server) handleEnvReorderPipelines(w http.ResponseWriter, r *http.Request) {
	if s.environments == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("environments store not configured"))
		return
	}
	name := r.PathValue("name")
	env, err := s.environments.GetByName(r.Context(), name)
	if err != nil {
		writeStorageErr(w, err, "environment not found")
		return
	}
	var body struct {
		PipelineIDs []string `json:"pipelineIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	cur := map[string]bool{}
	for _, p := range env.PipelineIDs {
		cur[p] = true
	}
	if len(body.PipelineIDs) != len(env.PipelineIDs) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("expected a permutation of %d pipeline ids, got %d", len(env.PipelineIDs), len(body.PipelineIDs)))
		return
	}
	seen := map[string]bool{}
	for _, p := range body.PipelineIDs {
		if !cur[p] {
			writeError(w, http.StatusBadRequest, fmt.Errorf("pipeline %q is not in this environment", p))
			return
		}
		if seen[p] {
			writeError(w, http.StatusBadRequest, fmt.Errorf("pipeline %q listed twice", p))
			return
		}
		seen[p] = true
	}
	env.PipelineIDs = body.PipelineIDs
	env.UpdatedAt = time.Now().UTC()
	if err := s.environments.Update(r.Context(), env); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}
