package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	restate "github.com/restatedev/sdk-go"
	"github.com/restatedev/sdk-go/ingress"

	"github.com/lyzrai/flow/pkg/models"
)

// RestateOrchestrator submits workflows to a Restate server via its HTTP ingress.
// Each node in the workflow becomes a journaled step that survives crashes.
type RestateOrchestrator struct {
	restateURL string
}

// NewRestateOrchestrator constructs an orchestrator pointed at restateURL
// (the Restate ingress URL, e.g. http://localhost:8081).
func NewRestateOrchestrator(restateURL string) *RestateOrchestrator {
	return &RestateOrchestrator{restateURL: restateURL}
}

// IngressURL returns the Restate ingress URL for external API calls
// (e.g., awakeable resolution from the resume HTTP handler).
func (o *RestateOrchestrator) IngressURL() string { return o.restateURL }

// Run submits a workflow synchronously and blocks until it finishes.
func (o *RestateOrchestrator) Run(ctx context.Context, req *RunRequest) (string, *models.ExecutionResult, error) {
	client := ingress.NewClient(o.restateURL)
	workflowID := generateWorkflowID()

	wfReq := &WorkflowRequest{
		RequestMeta: req.RequestMeta,
		Workflow:    req.Workflow,
		TriggerData: req.TriggerData,
	}

	result, err := ingress.Workflow[*WorkflowRequest, *models.ExecutionResult](
		client, "WorkflowExecutor", workflowID, "Run",
	).Request(ctx, wfReq)
	if err != nil {
		return workflowID, nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	result.ExecutionID = workflowID
	return workflowID, result, nil
}

// RunAsync submits a workflow and returns immediately with an execution ID.
func (o *RestateOrchestrator) RunAsync(ctx context.Context, req *RunRequest) (string, error) {
	client := ingress.NewClient(o.restateURL)
	workflowID := generateWorkflowID()

	wfReq := &WorkflowRequest{
		RequestMeta: req.RequestMeta,
		Workflow:    req.Workflow,
		TriggerData: req.TriggerData,
	}

	_, err := ingress.Workflow[*WorkflowRequest, *models.ExecutionResult](
		client, "WorkflowExecutor", workflowID, "Run",
	).Send(ctx, wfReq)
	if err != nil {
		return "", fmt.Errorf("async workflow submission failed: %w", err)
	}

	return workflowID, nil
}

// GetExecution returns the live status of a workflow execution.
// While running, it also probes for a pending approval via the shared handler.
func (o *RestateOrchestrator) GetExecution(ctx context.Context, executionID string) (*ExecutionStatus, error) {
	client := ingress.NewClient(o.restateURL)

	handle := ingress.WorkflowHandle[*models.ExecutionResult](client, "WorkflowExecutor", executionID)

	result, err := handle.Output(ctx)
	if err != nil {
		var notReady *ingress.InvocationNotReadyError
		if errors.As(err, &notReady) {
			status := &ExecutionStatus{ExecutionID: executionID, Status: "running"}
			approval, aErr := ingress.Workflow[restate.Void, *PendingApproval](
				client, "WorkflowExecutor", executionID, "GetPendingApproval",
			).Request(ctx, restate.Void{})
			if aErr == nil && approval != nil && approval.AwakeableID != "" {
				status.Status = "waiting_for_approval"
				status.PendingApproval = approval
			}
			return status, nil
		}
		var notFound *ingress.InvocationNotFoundError
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("execution %q not found", executionID)
		}
		return nil, fmt.Errorf("failed to get execution status: %w", err)
	}

	return &ExecutionStatus{
		ExecutionID: executionID,
		Status:      result.Status,
		Outputs:     result.Outputs,
		NodeOutputs: result.NodeOutputs,
		Errors:      result.Errors,
	}, nil
}

func generateWorkflowID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
