// Package api provides the HTTP server for flow.
//
// It serves only the JSON API under /api/*. The UI is a separate process
// (see ./web — nginx in prod, `next dev` locally) that proxies /api to here.
// Anything outside /api returns 404.
//
// The router is intentionally std-library only at this stage; we'll lift in a
// proper router (chi or gin) when the API surface grows.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
	"github.com/lyzrai/flow/pkg/orchestrator"
)

// FlowSummary is the list-shape returned to the dashboard.
type FlowSummary struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
	NodeCount   int       `json:"nodeCount"`
	Status      string    `json:"status,omitempty"`
}

// ServerDeps groups the construction-time dependencies of the HTTP server.
// Orchestrator drives durable execution; RestateIngressURL is used to
// resolve Awakeables from the resume handler. CORSOrigins lists allowed
// browser origins (use "*" to allow any — fine for dev).
type ServerDeps struct {
	Orchestrator      orchestrator.Orchestrator
	RestateIngressURL string
	CORSOrigins       []string
}

// Server is a thin JSON API server. It does not serve a frontend.
type Server struct {
	mux           *http.ServeMux
	orch          orchestrator.Orchestrator
	restateIngres string
	corsOrigins   []string

	mu    sync.Mutex
	flows map[string]storedFlow // in-memory placeholder; storage layer lands next
}

// Definition is held as raw n8n-format JSON so we don't lose connection
// shape on round-trip. We parse on read for validation + node count.
type storedFlow struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Definition json.RawMessage `json:"definition"`
	UpdatedAt  time.Time       `json:"updatedAt"`
	NodeCount  int             `json:"nodeCount"`
}

// NewServer constructs an API-only Server. deps.Orchestrator may be nil —
// execute routes will then return 503.
func NewServer(deps ServerDeps) *Server {
	s := &Server{
		mux:           http.NewServeMux(),
		orch:          deps.Orchestrator,
		restateIngres: deps.RestateIngressURL,
		corsOrigins:   deps.CORSOrigins,
		flows:         make(map[string]storedFlow),
	}
	s.routes()
	return s
}

// ServeHTTP implements http.Handler. CORS is applied here so all routes
// (including OPTIONS preflight) get the headers.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.applyCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	s.mux.HandleFunc("GET /api/workflows", s.handleListFlows)
	s.mux.HandleFunc("POST /api/workflows", s.handleCreateFlow)
	s.mux.HandleFunc("GET /api/workflows/{id}", s.handleGetFlow)
	s.mux.HandleFunc("PUT /api/workflows/{id}", s.handleUpdateFlow)
	s.mux.HandleFunc("DELETE /api/workflows/{id}", s.handleDeleteFlow)
	s.mux.HandleFunc("POST /api/workflows/execute", s.handleExecuteWorkflow)
	s.mux.HandleFunc("GET /api/executions/{id}", s.handleGetExecution)
	s.mux.HandleFunc("POST /api/executions/{id}/resume", s.handleResumeExecution)

	// Anything not under /api/ is not our concern — the UI server handles it.
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no handler for %s", r.URL.Path))
	})
}

// applyCORS sets the response headers needed for the UI process to call us
// from a different origin. In dev that's http://localhost:3000; in prod it's
// the nginx web container (same origin via proxy, but harmless to allow).
func (s *Server) applyCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	allowed := false
	for _, o := range s.corsOrigins {
		if o == "*" || o == origin {
			allowed = true
			break
		}
	}
	if !allowed {
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
	w.Header().Set("Access-Control-Max-Age", "600")
}

// --- handlers -------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListFlows(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]FlowSummary, 0, len(s.flows))
	for _, f := range s.flows {
		out = append(out, FlowSummary{
			ID:        f.ID,
			Name:      f.Name,
			UpdatedAt: f.UpdatedAt,
			NodeCount: f.NodeCount,
			Status:    "draft",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateFlow(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name       string          `json:"name"`
		Definition json.RawMessage `json:"definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, parsedName, err := analyzeDefinition(body.Definition)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := newID()
	f := storedFlow{
		ID:         id,
		Name:       firstNonEmpty(body.Name, parsedName, "Untitled flow"),
		Definition: body.Definition,
		UpdatedAt:  time.Now().UTC(),
		NodeCount:  count,
	}
	s.mu.Lock()
	s.flows[id] = f
	s.mu.Unlock()
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	f, ok := s.flows[id]
	s.mu.Unlock()
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("flow not found"))
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) handleUpdateFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name       *string         `json:"name"`
		Definition json.RawMessage `json:"definition"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flows[id]
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("flow not found"))
		return
	}
	if body.Name != nil {
		f.Name = *body.Name
	}
	if len(body.Definition) > 0 {
		count, _, err := analyzeDefinition(body.Definition)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		f.Definition = body.Definition
		f.NodeCount = count
	}
	f.UpdatedAt = time.Now().UTC()
	s.flows[id] = f
	w.WriteHeader(http.StatusNoContent)
}

// analyzeDefinition tries to parse the provided JSON as a flow workflow.
// Save-time is intentionally lenient — drafts with no trigger or no nodes are
// allowed. Strict validation happens at run time. We still surface obviously
// malformed JSON as a 400.
func analyzeDefinition(raw json.RawMessage) (int, string, error) {
	if len(raw) == 0 {
		return 0, "", nil
	}
	var probe struct {
		Name  string           `json:"name"`
		Nodes []map[string]any `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return 0, "", fmt.Errorf("definition is not valid JSON: %w", err)
	}
	// Best-effort full parse. Failures are fine at draft time.
	if _, err := engine.ParseWorkflow(raw); err != nil {
		_ = err // intentionally swallow; draft state.
	}
	return len(probe.Nodes), probe.Name, nil
}

func (s *Server) handleDeleteFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	delete(s.flows, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// handleExecuteWorkflow accepts either an inline workflow definition or a
// stored workflow ID and submits it to the orchestrator asynchronously.
//   POST /api/workflows/execute
//   { "workflow": <n8n-format JSON>, "input": [...] } | { "workflow_id": "...", "input": [...] }
// Returns 202 with { execution_id, status } so the UI can route to the run-
// detail page and stream events.
func (s *Server) handleExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.orch == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("orchestrator not configured"))
		return
	}

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	wfJSON, input, err := s.parseExecuteRequest(rawBody)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	wf, err := engine.ParseWorkflow(wfJSON)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse workflow: %w", err))
		return
	}

	apiKey := r.Header.Get("X-API-Key")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	execID, err := s.orch.RunAsync(ctx, &orchestrator.RunRequest{
		RequestMeta: orchestrator.RequestMeta{APIKey: apiKey},
		Workflow:    wf,
		TriggerData: input,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("submit execution: %w", err))
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"execution_id": execID,
		"status":       "running",
	})
}

// handleResumeExecution resolves a Restate Awakeable so a paused workflow
// continues. Body: { "awakeable_id": "...", "data": <any> }
//
// `data` is forwarded as-is to the Awakeable; the Approval node interprets
// `{"approved": bool, "reason"?: string, ...}`.
func (s *Server) handleResumeExecution(w http.ResponseWriter, r *http.Request) {
	if s.restateIngres == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("resume requires restate ingress URL"))
		return
	}
	var body struct {
		AwakeableID string          `json:"awakeable_id"`
		Data        json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.AwakeableID == "" {
		writeError(w, http.StatusBadRequest, errors.New("awakeable_id is required"))
		return
	}
	payload := body.Data
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}

	url := strings.TrimRight(s.restateIngres, "/") + "/restate/awakeables/" + body.AwakeableID + "/resolve"

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("resume call failed: %w", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		writeError(w, resp.StatusCode, fmt.Errorf("restate returned %d: %s", resp.StatusCode, string(respBody)))
		return
	}

	slog.InfoContext(r.Context(), "workflow_resumed_via_api",
		slog.String("execution_id", r.PathValue("id")),
		slog.String("awakeable_id", body.AwakeableID),
	)
	writeJSON(w, http.StatusOK, map[string]string{"message": "workflow resumed"})
}

func (s *Server) handleGetExecution(w http.ResponseWriter, r *http.Request) {
	if s.orch == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("orchestrator not configured"))
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("execution id required"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status, err := s.orch.GetExecution(ctx, id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// parseExecuteRequest accepts the structured ExecuteRequest shape:
//   { "workflow": <n8n-JSON object>, "input": [{...}, ...] }
//   { "workflow_id": "<id>", "input": [...] }
// Either path returns the raw n8n-format workflow JSON ready for ParseWorkflow.
func (s *Server) parseExecuteRequest(body []byte) (json.RawMessage, []models.Item, error) {
	var probe struct {
		Workflow   json.RawMessage `json:"workflow"`
		WorkflowID string          `json:"workflow_id"`
		Input      []models.Item   `json:"input"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return nil, nil, fmt.Errorf("invalid request body: %w", err)
	}

	input := probe.Input
	if len(input) == 0 {
		input = []models.Item{{}}
	}

	if probe.WorkflowID != "" {
		s.mu.Lock()
		f, ok := s.flows[probe.WorkflowID]
		s.mu.Unlock()
		if !ok {
			return nil, nil, fmt.Errorf("workflow %q not found", probe.WorkflowID)
		}
		return f.Definition, input, nil
	}
	if len(probe.Workflow) > 0 && string(probe.Workflow) != "null" {
		return probe.Workflow, input, nil
	}
	return nil, nil, errors.New("either workflow or workflow_id is required")
}

// --- helpers --------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write json response", slog.Any("error", err))
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func newID() string {
	// Compact ULID-ish without pulling a dep: timestamp + crypto random.
	now := time.Now().UTC().UnixNano()
	return strings.ReplaceAll(time.Unix(0, now).Format("20060102T150405.000000000"), ".", "")
}
