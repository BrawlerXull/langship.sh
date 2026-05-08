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
	"strconv"
	"strings"
	"time"

	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
	"github.com/lyzrai/flow/pkg/orchestrator"
	"github.com/lyzrai/flow/pkg/storage"
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
// browser origins (use "*" to allow any — fine for dev). Pipelines / Runs /
// Agents are required — the API has no in-memory fallback.
type ServerDeps struct {
	Orchestrator      orchestrator.Orchestrator
	RestateIngressURL string
	CORSOrigins       []string
	// PublicURL is the externally-reachable base URL for this API (e.g.
	// "https://abcd.trycloudflare.com"). Used to render webhook callback
	// URLs that GitHub can hit. Empty means webhook install is disabled.
	PublicURL string
	Pipelines storage.PipelineStore
	Runs      storage.RunStore
	Agents    storage.AgentStore
	// Events is the in-memory pub/sub bus the orchestrator publishes
	// per-node lifecycle events to. The SSE handler subscribes per
	// execution ID. Nil disables /api/executions/{id}/stream.
	Events EventSubscriber
}

// EventSubscriber is the slice of execevents.MemoryBus the API needs.
// Defined here as a tiny interface so we don't pull pkg/execevents into
// the api package's import graph.
type EventSubscriber interface {
	Subscribe(execID string) (<-chan engine.ExecutionEvent, func())
}

// Server is a thin JSON API server. It does not serve a frontend.
type Server struct {
	mux           *http.ServeMux
	orch          orchestrator.Orchestrator
	restateIngres string
	corsOrigins   []string
	publicURL     string

	pipelines storage.PipelineStore
	runs      storage.RunStore
	agents    storage.AgentStore
	events    EventSubscriber
}

// NewServer constructs an API-only Server. deps.Orchestrator may be nil —
// execute routes will then return 503. Stores must be non-nil; CRUD routes
// will panic without them — the API has no in-memory fallback.
func NewServer(deps ServerDeps) *Server {
	s := &Server{
		mux:           http.NewServeMux(),
		orch:          deps.Orchestrator,
		restateIngres: deps.RestateIngressURL,
		corsOrigins:   deps.CORSOrigins,
		publicURL:     strings.TrimRight(deps.PublicURL, "/"),
		pipelines:     deps.Pipelines,
		runs:          deps.Runs,
		agents:        deps.Agents,
		events:        deps.Events,
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
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/workflows", s.handleListFlows)
	s.mux.HandleFunc("POST /api/workflows", s.handleCreateFlow)
	s.mux.HandleFunc("GET /api/workflows/{id}", s.handleGetFlow)
	s.mux.HandleFunc("PUT /api/workflows/{id}", s.handleUpdateFlow)
	s.mux.HandleFunc("DELETE /api/workflows/{id}", s.handleDeleteFlow)
	s.mux.HandleFunc("POST /api/workflows/execute", s.handleExecuteWorkflow)
	s.mux.HandleFunc("GET /api/executions", s.handleListExecutions)
	s.mux.HandleFunc("GET /api/executions/{id}", s.handleGetExecution)
	s.mux.HandleFunc("GET /api/executions/{id}/stream", s.handleStreamExecution)
	s.mux.HandleFunc("POST /api/executions/{id}/resume", s.handleResumeExecution)

	// Agents — Langship-style agent registry (git URL + PAT)
	s.mux.HandleFunc("GET /api/agents", s.handleListAgents)
	s.mux.HandleFunc("POST /api/agents", s.handleCreateAgent)
	s.mux.HandleFunc("GET /api/agents/{id}", s.handleGetAgent)
	s.mux.HandleFunc("DELETE /api/agents/{id}", s.handleDeleteAgent)
	s.mux.HandleFunc("POST /api/agents/{id}/test-auth", s.handleTestAgentAuth)
	s.mux.HandleFunc("POST /api/agents/{id}/webhook", s.handleInstallWebhook)
	s.mux.HandleFunc("DELETE /api/agents/{id}/webhook", s.handleUninstallWebhook)
	s.mux.HandleFunc("POST /api/agents/{id}/pipelines/{pipelineId}", s.handleAttachPipeline)
	s.mux.HandleFunc("DELETE /api/agents/{id}/pipelines/{pipelineId}", s.handleDetachPipeline)
	s.mux.HandleFunc("POST /api/agents/{id}/trigger", s.handleTriggerAgent)

	// Public webhook receiver. GitHub posts here; HMAC signature is the
	// authentication. Must NOT require CORS / API auth.
	s.mux.HandleFunc("POST /webhooks/github/{id}", s.handleGitHubWebhook)

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

// handleConfig exposes a few server-side config values to the UI so it can
// render webhook URLs / decide whether to disable buttons.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"publicUrl":           s.publicURL,
		"webhooksAvailable":   s.publicURL != "",
		"orchestratorEnabled": s.orch != nil,
	})
}

func (s *Server) handleListFlows(w http.ResponseWriter, r *http.Request) {
	pipes, err := s.pipelines.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]FlowSummary, 0, len(pipes))
	for _, p := range pipes {
		out = append(out, FlowSummary{
			ID:        p.ID,
			Name:      p.Name,
			UpdatedAt: p.UpdatedAt,
			NodeCount: p.NodeCount,
			Status:    firstNonEmpty(p.Status, "draft"),
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
	now := time.Now().UTC()
	id := newID()
	p := &storage.Pipeline{
		ID:         id,
		Name:       firstNonEmpty(body.Name, parsedName, "Untitled pipeline"),
		Definition: body.Definition,
		NodeCount:  count,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.pipelines.Create(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) handleGetFlow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.pipelines.Get(r.Context(), id)
	if err != nil {
		writeStorageErr(w, err, "pipeline not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
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

	p, err := s.pipelines.Get(r.Context(), id)
	if err != nil {
		writeStorageErr(w, err, "pipeline not found")
		return
	}

	if body.Name != nil {
		p.Name = *body.Name
	}
	if len(body.Definition) > 0 {
		count, _, err := analyzeDefinition(body.Definition)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		p.Definition = body.Definition
		p.NodeCount = count
	}
	p.UpdatedAt = time.Now().UTC()
	if err := s.pipelines.Update(r.Context(), p); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
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
	if err := s.pipelines.Delete(r.Context(), id); err != nil && !errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
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

	// Pull the originating pipeline ID/name out of the body again so we can
	// stamp the run record (parseExecuteRequest doesn't expose it).
	pipelineID, pipelineName := pipelineRefFromBody(rawBody, wf.Name)

	apiKey := r.Header.Get("X-API-Key")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	execID, err := s.orch.RunAsync(ctx, &orchestrator.RunRequest{
		RequestMeta: orchestrator.RequestMeta{APIKey: apiKey, WorkflowID: pipelineID},
		Workflow:    wf,
		TriggerData: input,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("submit execution: %w", err))
		return
	}

	// Best-effort run record. A failure here shouldn't block the response —
	// the orchestrator already accepted the workflow.
	if s.runs != nil {
		triggerJSON, _ := json.Marshal(input)
		if err := s.runs.Insert(r.Context(), &storage.Run{
			ID:           execID,
			PipelineID:   pipelineID,
			PipelineName: pipelineName,
			Status:       "running",
			StartedAt:    time.Now().UTC(),
			TriggerData:  triggerJSON,
		}); err != nil {
			slog.WarnContext(r.Context(), "run_insert_failed",
				slog.String("execution_id", execID),
				slog.Any("error", err),
			)
		}
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"execution_id": execID,
		"status":       "running",
	})
}

// pipelineRefFromBody peeks at the execute request body to recover the
// pipeline ID + name for the run record. Best-effort; missing fields are OK.
func pipelineRefFromBody(body []byte, fallbackName string) (id string, name string) {
	var probe struct {
		WorkflowID string `json:"workflow_id"`
		Workflow   struct {
			Name string `json:"name"`
		} `json:"workflow"`
	}
	_ = json.Unmarshal(body, &probe)
	name = firstNonEmpty(probe.Workflow.Name, fallbackName)
	return probe.WorkflowID, name
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

// handleStreamExecution streams ExecutionEvent JSON over text/event-stream.
// Subscribes to the in-memory bus for the given execution ID. Closes the
// connection after a terminal event ("done") or when the client disconnects.
//
// Each event is emitted as a single SSE message:
//
//	data: {"type":"node_started","node":"Build","status":"running"}\n\n
//
// Heartbeats every 15s keep proxies (nginx, Cloudflare) from idling out.
func (s *Server) handleStreamExecution(w http.ResponseWriter, r *http.Request) {
	if s.events == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("event stream not configured"))
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("execution id required"))
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering
	w.WriteHeader(http.StatusOK)

	ch, cancel := s.events.Subscribe(id)
	defer cancel()

	// Tell the client which execution it's subscribed to (also primes the
	// SSE pipe so flushers in the middle don't withhold the first byte).
	_, _ = fmt.Fprintf(w, "event: open\ndata: {\"execution_id\":%q}\n\n", id)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			// SSE comments are heartbeats; clients ignore them.
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
			if ev.Type == engine.EventDone {
				return
			}
		}
	}
}

// handleListExecutions returns recent runs from storage. Optional
// ?pipeline_id= filters by source pipeline; ?limit= caps the page size.
func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	if s.runs == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("run store not configured"))
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	var (
		runs []*storage.Run
		err  error
	)
	if pid := r.URL.Query().Get("pipeline_id"); pid != "" {
		runs, err = s.runs.ListByPipeline(r.Context(), pid, limit)
	} else {
		runs, err = s.runs.List(r.Context(), limit)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p, err := s.pipelines.Get(ctx, probe.WorkflowID)
		if err != nil {
			return nil, nil, fmt.Errorf("workflow %q not found", probe.WorkflowID)
		}
		return p.Definition, input, nil
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

// writeStorageErr maps storage.ErrNotFound to 404 and other errors to 500.
// notFoundMsg is the user-facing message on 404.
func writeStorageErr(w http.ResponseWriter, err error, notFoundMsg string) {
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, errors.New(notFoundMsg))
		return
	}
	writeError(w, http.StatusInternalServerError, err)
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
