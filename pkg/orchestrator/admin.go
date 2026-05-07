package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HealthCheckRestate verifies the Restate ingress is reachable. The flow
// binary fails fast at startup if this returns an error.
func HealthCheckRestate(ingressURL string) error {
	client := &http.Client{Timeout: 3 * time.Second}
	r, err := client.Get(ingressURL + "/restate/health")
	if err != nil {
		return fmt.Errorf("restate ingress unreachable at %s: %w", ingressURL, err)
	}
	defer r.Body.Close()
	if r.StatusCode != http.StatusOK {
		return fmt.Errorf("restate ingress returned %d at %s", r.StatusCode, ingressURL)
	}
	return nil
}

// RegisterDeployment registers this service with the Restate admin API so
// Restate knows where to call back for the WorkflowExecutor handler.
//
// adminURL  — Restate admin endpoint (default http://localhost:9070)
// deployURI — how Restate reaches this service (e.g. http://flow:9080)
func RegisterDeployment(ctx context.Context, adminURL, deployURI string) error {
	body, _ := json.Marshal(map[string]any{"uri": deployURI, "force": true})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, adminURL+"/deployments", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build admin request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	r, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("restate admin call failed: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode >= 300 {
		respBody, _ := io.ReadAll(r.Body)
		return fmt.Errorf("restate admin returned %d: %s", r.StatusCode, string(respBody))
	}
	return nil
}

// RegisterDeploymentWithRetry retries registration until it succeeds or ctx
// is cancelled. Used at startup because the Restate cluster may take a moment
// to become ready relative to flow.
func RegisterDeploymentWithRetry(ctx context.Context, adminURL, deployURI string, maxAttempts int, interval time.Duration) error {
	var last error
	for i := 0; i < maxAttempts; i++ {
		if err := RegisterDeployment(ctx, adminURL, deployURI); err == nil {
			return nil
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
	return fmt.Errorf("registration failed after %d attempts: %w", maxAttempts, last)
}
