package executors

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lyzrai/flow/pkg/awsdeploy"
	"github.com/lyzrai/flow/pkg/engine"
	"github.com/lyzrai/flow/pkg/models"
	"github.com/lyzrai/flow/pkg/storage"
)

// DeployExecutor deploys an already-built container image to a managed
// agent runtime. Today only the AWS Bedrock AgentCore target is wired
// (target=agentcore); k8s and Vertex come later (the form / catalog show
// them as "coming soon").
//
// On success, every output item gets a `__deploy` summary that includes
// the invokeUrl — the public HTTPS endpoint clients hit to talk to the
// deployed agent. A final node-log line restates that URL so it is
// visible without expanding the run output.
//
// Image-source resolution order:
//  1. node.parameters.image                  (explicit override)
//  2. upstream `__push.copies[0].imageRef`   (Push node — the canonical case)
//  3. upstream `__push.dst`                  (legacy single-target push shape)
//  4. upstream `__build.image`               (Build node, no Push step)
//
// AWS creds: the agent record must carry awsRegion + awsAccountId +
// awsCrossAccountRoleArn. The Flow host's own AWS identity (env / IRSA /
// instance profile) calls STS:AssumeRole on that cross-account role; the
// resulting temp creds drive ECR / IAM / AgentCore.
type DeployExecutor struct {
	Agents      storage.AgentStore
	Credentials storage.CredentialStore // optional — global credential pool fallback
}

func (e *DeployExecutor) Execute(ctx context.Context, node models.NodeDef, inputs [][]models.Item, _ *engine.ExecutionContext) (map[int][]models.Item, error) {
	if e.Agents == nil {
		return nil, errors.New("deploy: AgentStore not configured")
	}
	logger := engine.NodeLoggerFromContext(ctx)

	target := strings.ToLower(strParam(node.Parameters, "target", "agentcore"))
	switch target {
	case "agentcore":
		// supported below
	case "k8s", "kubernetes", "vertex", "":
		return nil, fmt.Errorf("deploy: target %q not yet implemented (only \"agentcore\" today)", target)
	default:
		return nil, fmt.Errorf("deploy: unknown target %q", target)
	}

	trigger := firstItem(inputs)
	agentID, _ := trigger["agentId"].(string)
	if agentID == "" {
		return nil, errors.New("deploy: trigger payload missing agentId")
	}
	a, err := e.Agents.Get(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("deploy: load agent %q: %w", agentID, err)
	}
	credName := strParam(node.Parameters, "credentialName", "aws")
	cred, credScope, err := e.lookupCredential(ctx, a, credName)
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}
	logger.Log(fmt.Sprintf("[deploy] using %s credential %q", credScope, credName))
	if cred.Type != storage.CredentialAWS {
		return nil, fmt.Errorf("deploy: credential %q is type %q; target=agentcore needs an aws credential", credName, cred.Type)
	}
	if cred.AwsRegion == "" || cred.AwsAccountID == "" || cred.AwsCrossAccountRoleArn == "" {
		return nil, fmt.Errorf("deploy: aws credential %q is missing region / accountId / crossAccountRoleArn", credName)
	}

	image := resolveDeployImage(node.Parameters, inputs)
	if image == "" {
		return nil, errors.New("deploy: no image to deploy — Push must run upstream, or set `image` on the node")
	}

	runtimeName := strParam(node.Parameters, "runtimeName", "")
	if runtimeName == "" {
		runtimeName = deriveRuntimeName(a)
	}
	envVars := resolveEnvVars(node.Parameters)
	timeoutSec := intParam(node.Parameters, "timeoutSeconds", 600)
	hardCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	logger.Log(fmt.Sprintf("[deploy:agentcore] credential=%s account=%s region=%s runtime=%s image=%s",
		cred.Name, cred.AwsAccountID, cred.AwsRegion, runtimeName, image))

	awsCfg := awsdeploy.Config{
		Region:              cred.AwsRegion,
		AccountID:           cred.AwsAccountID,
		CrossAccountRoleArn: cred.AwsCrossAccountRoleArn,
	}
	creds, err := awsdeploy.AssumeCustomer(hardCtx, awsCfg)
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}
	customerCfg, err := awsdeploy.CustomerConfig(hardCtx, awsCfg, creds)
	if err != nil {
		return nil, fmt.Errorf("deploy: build customer aws config: %w", err)
	}

	// Bootstrap (idempotent): ECR repo + shared agentcore-runtime-role.
	// Repo name follows the JS reference: lowercased "agentcore-<runtime>".
	ecrRepoName := strings.ToLower("agentcore-" + runtimeName)
	if _, err := awsdeploy.EnsureECRRepository(hardCtx, customerCfg, cred.AwsAccountID, cred.AwsRegion, ecrRepoName, logger); err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}
	runtimeRoleArn, err := awsdeploy.EnsureAgentCoreRuntimeRole(hardCtx, customerCfg, cred.AwsAccountID, cred.AwsRegion, logger)
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}

	// Create or update the AgentCore runtime + wait for endpoint READY.
	res, err := awsdeploy.CreateOrUpdateAgentRuntime(
		hardCtx,
		awsCfg,
		creds,
		runtimeName,
		image,
		runtimeRoleArn,
		envVars,
		time.Duration(timeoutSec)*time.Second,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}

	// Make the endpoint impossible to miss in the run timeline.
	logger.Log("══════════════════════════════════════════════════════════════════")
	logger.Log(fmt.Sprintf("✅ Deploy complete. Invoke URL: %s", res.InvokeURL))
	logger.Log(fmt.Sprintf("   Agent runtime: %s", res.AgentRuntimeArn))
	logger.Log(fmt.Sprintf("   Endpoint ARN : %s", res.EndpointArn))
	logger.Log("══════════════════════════════════════════════════════════════════")

	summary := map[string]any{
		"target":          "agentcore",
		"agentId":         agentID,
		"agentName":       a.Name,
		"credentialName":  cred.Name,
		"credentialScope": credScope,
		"region":          cred.AwsRegion,
		"accountId":       cred.AwsAccountID,
		"runtimeName":     runtimeName,
		"image":           image,
		"agentRuntimeId":  res.AgentRuntimeID,
		"agentRuntimeArn": res.AgentRuntimeArn,
		"endpointArn":     res.EndpointArn,
		"invokeUrl":       res.InvokeURL,
		"runtimeRoleArn":  runtimeRoleArn,
		"finished_at":     time.Now().UTC(),
	}

	out := make([]models.Item, 0)
	for _, in := range inputs {
		for _, it := range in {
			ci := copyItem(it)
			ci["__deploy"] = summary
			out = append(out, ci)
		}
	}
	if len(out) == 0 {
		out = append(out, models.Item{"__deploy": summary})
	}
	return map[int][]models.Item{0: out}, nil
}

// resolveDeployImage walks the upstream items in priority order (Push >
// Build) and picks the first image ref it finds. Node-level `image`
// param wins over both. Returns "" if nothing is set.
func resolveDeployImage(params map[string]any, inputs [][]models.Item) string {
	if v := strParam(params, "image", ""); v != "" {
		return v
	}
	for _, in := range inputs {
		for _, it := range in {
			// __push.copies[0].imageRef
			if push, ok := it["__push"].(map[string]any); ok {
				if copies, ok := push["copies"].([]any); ok && len(copies) > 0 {
					if first, ok := copies[0].(map[string]any); ok {
						if ref, ok := first["imageRef"].(string); ok && ref != "" {
							return ref
						}
					}
				}
				// Legacy single-target shape: __push.dst
				if dst, ok := push["dst"].(string); ok && dst != "" {
					return dst
				}
			}
			// __build.image
			if build, ok := it["__build"].(map[string]any); ok {
				if ref, ok := build["image"].(string); ok && ref != "" {
					return ref
				}
			}
		}
	}
	return ""
}

// resolveEnvVars accepts either a map[string]any (UI form's natural shape)
// or a JSON string. Empty values are dropped — AgentCore doesn't accept
// empty-string values on environmentVariables.
func resolveEnvVars(params map[string]any) map[string]string {
	out := map[string]string{}
	raw, ok := params["envVars"]
	if !ok {
		return out
	}
	if m, ok := raw.(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok && s != "" {
				out[k] = s
			}
		}
	}
	return out
}

// deriveRuntimeName: agent.Name is "owner/repo" — AgentCore enforces
// [a-zA-Z0-9_]{1,48}, so we replace `/` with `_` and let the adapter's
// sanitizer handle anything else. Empty agent name falls back to the ID.
func deriveRuntimeName(a *storage.Agent) string {
	n := a.Name
	if n == "" {
		n = a.ID
	}
	return strings.ReplaceAll(n, "/", "_")
}

// lookupCredential resolves a credential by name with the documented
// precedence: agent-level override first, then the global pool. Returns
// the resolved credential, a scope label ("agent" | "global") for the
// __deploy summary, and a typed error.
//
// We require the credential to be of type AWS for now since that's the
// only target the executor implements; tightening here gives a clearer
// error than letting empty fields blow up further down.
func (e *DeployExecutor) lookupCredential(ctx context.Context, a *storage.Agent, name string) (storage.Credential, string, error) {
	if c, ok := a.LookupCredential(name); ok {
		if c.Type != storage.CredentialAWS {
			return storage.Credential{}, "", fmt.Errorf("agent credential %q is type %q; target=agentcore needs an aws credential", name, c.Type)
		}
		if c.AwsRegion == "" || c.AwsAccountID == "" || c.AwsCrossAccountRoleArn == "" {
			return storage.Credential{}, "", fmt.Errorf("agent aws credential %q is missing region / accountId / crossAccountRoleArn", name)
		}
		return c, "agent", nil
	}
	if e.Credentials == nil {
		return storage.Credential{}, "", fmt.Errorf("no credential %q on agent and global credential store not configured", name)
	}
	gc, err := e.Credentials.GetByName(ctx, name)
	if err != nil {
		return storage.Credential{}, "", fmt.Errorf("no credential %q on agent or in the global pool: %w", name, err)
	}
	if gc.Type != storage.CredentialAWS {
		return storage.Credential{}, "", fmt.Errorf("global credential %q is type %q; target=agentcore needs an aws credential", name, gc.Type)
	}
	if gc.AwsRegion == "" || gc.AwsAccountID == "" || gc.AwsCrossAccountRoleArn == "" {
		return storage.Credential{}, "", fmt.Errorf("global aws credential %q is missing region / accountId / crossAccountRoleArn", name)
	}
	return *gc, "global", nil
}
