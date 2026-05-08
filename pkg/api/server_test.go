package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lyzrai/flow/pkg/models"
	"github.com/lyzrai/flow/pkg/orchestrator"
	"github.com/lyzrai/flow/pkg/storage"
)

// newTestServer returns a Server with in-memory stores wired up. Tests pass
// extra fields (Orchestrator, RestateIngressURL) that override the defaults.
func newTestServer(deps ServerDeps) *Server {
	mem := storage.NewMemory()
	if deps.Pipelines == nil {
		deps.Pipelines = mem.Pipelines()
	}
	if deps.Runs == nil {
		deps.Runs = mem.Runs()
	}
	if deps.Agents == nil {
		deps.Agents = mem.Agents()
	}
	return NewServer(deps)
}

func TestHealth_returns200(t *testing.T) {
	srv := newTestServer(ServerDeps{})
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body: got %v", body)
	}
}

func TestWorkflowCRUD_roundTrip(t *testing.T) {
	srv := newTestServer(ServerDeps{})

	// Create
	wf := map[string]any{
		"name": "round-trip",
		"definition": map[string]any{
			"name": "round-trip",
			"nodes": []any{
				map[string]any{
					"id":         "1",
					"name":       "Trigger",
					"type":       "n8n-nodes-base.manualTrigger",
					"parameters": map[string]any{},
					"position":   []any{0, 0},
				},
			},
			"connections": map[string]any{},
		},
	}
	body, _ := json.Marshal(wf)
	r := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: got %d (%s)", w.Code, w.Body.String())
	}
	var created struct{ ID string `json:"id"` }
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("create: missing id")
	}

	// List
	r = httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("list: got %d", w.Code)
	}
	var list []FlowSummary
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("list body: %v", err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list: got %+v", list)
	}
	if list[0].NodeCount != 1 {
		t.Fatalf("nodeCount: expected 1, got %d", list[0].NodeCount)
	}
	if list[0].Name != "round-trip" {
		t.Fatalf("name: got %q", list[0].Name)
	}

	// Get
	r = httptest.NewRequest(http.MethodGet, "/api/workflows/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("get: got %d", w.Code)
	}

	// Update name only
	upd, _ := json.Marshal(map[string]any{"name": "renamed"})
	r = httptest.NewRequest(http.MethodPut, "/api/workflows/"+created.ID, bytes.NewReader(upd))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("update: got %d (%s)", w.Code, w.Body.String())
	}

	// Verify rename via list
	r = httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if list[0].Name != "renamed" {
		t.Fatalf("rename: got %q", list[0].Name)
	}

	// Delete
	r = httptest.NewRequest(http.MethodDelete, "/api/workflows/"+created.ID, nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: got %d", w.Code)
	}

	// Empty list
	r = httptest.NewRequest(http.MethodGet, "/api/workflows", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("expected empty list after delete, got %+v", list)
	}
}

func TestExecute_requiresOrchestrator(t *testing.T) {
	srv := newTestServer(ServerDeps{})
	body, _ := json.Marshal(map[string]any{
		"workflow": map[string]any{
			"name":        "x",
			"nodes":       []any{map[string]any{"id": "1", "name": "T", "type": "n8n-nodes-base.manualTrigger", "parameters": map[string]any{}, "position": []any{0, 0}}},
			"connections": map[string]any{},
		},
	})
	r := httptest.NewRequest(http.MethodPost, "/api/workflows/execute", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 when orchestrator nil (body=%s)", w.Code, w.Body.String())
	}
}

// stubOrch is an Orchestrator that records the last submitted workflow.
type stubOrch struct {
	lastWorkflow *models.WorkflowDefinition
	lastTrigger  []models.Item
	execID       string
	statusOut    *orchestrator.ExecutionStatus
	statusErr    error
}

func (s *stubOrch) Run(_ context.Context, req *orchestrator.RunRequest) (string, *models.ExecutionResult, error) {
	s.lastWorkflow = req.Workflow
	s.lastTrigger = req.TriggerData
	return s.execID, nil, nil
}
func (s *stubOrch) RunAsync(_ context.Context, req *orchestrator.RunRequest) (string, error) {
	s.lastWorkflow = req.Workflow
	s.lastTrigger = req.TriggerData
	return s.execID, nil
}
func (s *stubOrch) GetExecution(_ context.Context, _ string) (*orchestrator.ExecutionStatus, error) {
	if s.statusErr != nil {
		return nil, s.statusErr
	}
	return s.statusOut, nil
}

func TestExecute_inlineWorkflow_callsOrchestrator(t *testing.T) {
	orch := &stubOrch{execID: "exec-123"}
	srv := newTestServer(ServerDeps{Orchestrator: orch})

	body := []byte(`{"workflow":{"name":"x","nodes":[{"id":"1","name":"T","type":"n8n-nodes-base.manualTrigger","parameters":{},"position":[0,0]}],"connections":{}},"input":[{"k":"v"}]}`)
	r := httptest.NewRequest(http.MethodPost, "/api/workflows/execute", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusAccepted {
		t.Fatalf("got %d (%s), want 202", w.Code, w.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["execution_id"] != "exec-123" {
		t.Fatalf("execution_id: got %q", resp["execution_id"])
	}
	if orch.lastWorkflow == nil {
		t.Fatal("orchestrator was not called")
	}
	if orch.lastWorkflow.Name != "x" {
		t.Fatalf("workflow name not parsed: %+v", orch.lastWorkflow)
	}
	if len(orch.lastTrigger) != 1 || orch.lastTrigger[0]["k"] != "v" {
		t.Fatalf("trigger data not threaded: %+v", orch.lastTrigger)
	}
}

func TestExecute_byWorkflowID(t *testing.T) {
	orch := &stubOrch{execID: "exec-7"}
	srv := newTestServer(ServerDeps{Orchestrator: orch})

	// Pre-create a workflow.
	body, _ := json.Marshal(map[string]any{
		"name": "stored",
		"definition": map[string]any{
			"name":        "stored",
			"nodes":       []any{map[string]any{"id": "1", "name": "T", "type": "n8n-nodes-base.manualTrigger", "parameters": map[string]any{}, "position": []any{0, 0}}},
			"connections": map[string]any{},
		},
	})
	r := httptest.NewRequest(http.MethodPost, "/api/workflows", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	var created struct{ ID string `json:"id"` }
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	exec, _ := json.Marshal(map[string]any{"workflow_id": created.ID})
	r = httptest.NewRequest(http.MethodPost, "/api/workflows/execute", bytes.NewReader(exec))
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusAccepted {
		t.Fatalf("got %d (%s)", w.Code, w.Body.String())
	}
	if orch.lastWorkflow == nil || orch.lastWorkflow.Name != "stored" {
		t.Fatalf("by-id execute did not load stored workflow: %+v", orch.lastWorkflow)
	}
}

func TestExecute_missing_workflow_400(t *testing.T) {
	srv := newTestServer(ServerDeps{Orchestrator: &stubOrch{}})
	body := []byte(`{}`)
	r := httptest.NewRequest(http.MethodPost, "/api/workflows/execute", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d", w.Code)
	}
}

func TestGetExecution_returnsOrchStatus(t *testing.T) {
	want := &orchestrator.ExecutionStatus{ExecutionID: "abc", Status: "success"}
	srv := newTestServer(ServerDeps{Orchestrator: &stubOrch{statusOut: want}})

	r := httptest.NewRequest(http.MethodGet, "/api/executions/abc", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d (%s)", w.Code, w.Body.String())
	}
	var got orchestrator.ExecutionStatus
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.ExecutionID != "abc" || got.Status != "success" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGetExecution_notFound(t *testing.T) {
	srv := newTestServer(ServerDeps{Orchestrator: &stubOrch{statusErr: errors.New("nope")}})
	r := httptest.NewRequest(http.MethodGet, "/api/executions/abc", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d", w.Code)
	}
}

func TestResume_proxiesToRestate(t *testing.T) {
	got := struct {
		path        string
		body        []byte
		contentType string
	}{}
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.contentType = r.Header.Get("Content-Type")
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer stub.Close()

	srv := newTestServer(ServerDeps{RestateIngressURL: stub.URL})
	body, _ := json.Marshal(map[string]any{
		"awakeable_id": "sign_xyz",
		"data":         map[string]any{"approved": true},
	})
	r := httptest.NewRequest(http.MethodPost, "/api/executions/exec-1/resume", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("resume: got %d (%s)", w.Code, w.Body.String())
	}
	if got.path != "/restate/awakeables/sign_xyz/resolve" {
		t.Errorf("proxied path: got %q", got.path)
	}
	if !strings.Contains(string(got.body), `"approved":true`) {
		t.Errorf("proxied body: got %q", string(got.body))
	}
	if got.contentType != "application/json" {
		t.Errorf("content-type: got %q", got.contentType)
	}
}

func TestResume_requiresAwakeableID(t *testing.T) {
	srv := newTestServer(ServerDeps{RestateIngressURL: "http://localhost"})
	body, _ := json.Marshal(map[string]any{"awakeable_id": "", "data": map[string]any{}})
	r := httptest.NewRequest(http.MethodPost, "/api/executions/exec-1/resume", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got %d", w.Code)
	}
}

func TestResume_unwiredIngress503(t *testing.T) {
	srv := newTestServer(ServerDeps{}) // no RestateIngressURL
	body, _ := json.Marshal(map[string]any{"awakeable_id": "sign_x", "data": map[string]any{}})
	r := httptest.NewRequest(http.MethodPost, "/api/executions/exec-1/resume", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", w.Code)
	}
}

func TestNonAPI_returns404(t *testing.T) {
	// API server no longer hosts the SPA — the UI is a separate process.
	srv := newTestServer(ServerDeps{})
	for _, path := range []string{"/", "/flows/abc", "/runs/123"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: got %d, want 404", path, w.Code)
		}
	}
}

func TestUnknownAPIRoute_404(t *testing.T) {
	srv := newTestServer(ServerDeps{})
	r := httptest.NewRequest(http.MethodGet, "/api/does-not-exist", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d", w.Code)
	}
}
