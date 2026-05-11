// Package awsdeploy ports the AWS pieces of agent-deploy/build_service:
// cross-account STS AssumeRole, idempotent ECR + AgentCore-runtime-role
// bootstrap, and AgentCore control-plane create-or-update + endpoint
// readiness polling. It is the AWS-target adapter for the Flow Deploy
// node (see pkg/executors/deploy.go).
//
// AWS does not (yet) ship a Go SDK client for bedrock-agentcore-control,
// so AgentCore calls are raw HTTPS signed with SigV4 — the same approach
// the JS reference uses.
package awsdeploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

// Logger is the minimal sink the executor's per-node logger satisfies.
// Pass `engine.NodeLoggerFromContext(ctx)` from the caller.
type Logger interface {
	Log(line string)
}

type nopLogger struct{}

func (nopLogger) Log(string) {}

// loggerOr returns lg if non-nil, else a no-op.
func loggerOr(lg Logger) Logger {
	if lg == nil {
		return nopLogger{}
	}
	return lg
}

// Config is everything the deploy executor passes once per call.
type Config struct {
	Region              string
	AccountID           string
	CrossAccountRoleArn string // role in the customer account; this host assumes it
}

// AssumeCustomer returns AWS creds for the customer account by calling
// STS AssumeRole on the configured cross-account role. The Flow host's
// own creds (env / IRSA / instance profile) authenticate the AssumeRole.
func AssumeCustomer(ctx context.Context, c Config) (aws.CredentialsProvider, error) {
	if c.Region == "" || c.CrossAccountRoleArn == "" {
		return nil, errors.New("awsdeploy: region and crossAccountRoleArn are required")
	}
	hostCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(c.Region))
	if err != nil {
		return nil, fmt.Errorf("load host AWS config: %w", err)
	}
	stsCli := sts.NewFromConfig(hostCfg)
	prov := stscreds.NewAssumeRoleProvider(stsCli, c.CrossAccountRoleArn, func(o *stscreds.AssumeRoleOptions) {
		o.RoleSessionName = "flow-deploy"
		o.Duration = time.Hour
	})
	// Force one resolution up-front so we surface auth errors here, not on
	// the first AWS call inside an executor.
	if _, err := prov.Retrieve(ctx); err != nil {
		return nil, fmt.Errorf("assume role %s: %w", c.CrossAccountRoleArn, err)
	}
	return aws.NewCredentialsCache(prov), nil
}

// CustomerConfig is an aws.Config with the cross-account creds attached.
func CustomerConfig(ctx context.Context, c Config, creds aws.CredentialsProvider) (aws.Config, error) {
	return config.LoadDefaultConfig(ctx,
		config.WithRegion(c.Region),
		config.WithCredentialsProvider(creds),
	)
}

// EnsureECRRepository creates the repo if missing. Idempotent — already-
// exists is treated as success. Returns the repo URI (registry/name).
func EnsureECRRepository(ctx context.Context, awsCfg aws.Config, accountID, region, repoName string, lg Logger) (string, error) {
	lg = loggerOr(lg)
	cli := ecr.NewFromConfig(awsCfg)
	scanOnPush := true
	_, err := cli.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repoName),
		ImageScanningConfiguration: &ecrtypes.ImageScanningConfiguration{
			ScanOnPush: scanOnPush,
		},
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "RepositoryAlreadyExistsException" {
			lg.Log(fmt.Sprintf("[awsdeploy] ECR repo %s already exists", repoName))
		} else {
			return "", fmt.Errorf("create ECR repo %s: %w", repoName, err)
		}
	} else {
		lg.Log(fmt.Sprintf("[awsdeploy] ECR repo %s created", repoName))
	}
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com/%s", accountID, region, repoName), nil
}

// EnsureAgentCoreRuntimeRole creates the shared `agentcore-runtime-role`
// (per the JS reference) if missing, and attaches a runtime policy. The
// role's trust policy allows bedrock-agentcore.amazonaws.com to assume it,
// scoped to the customer account. Idempotent.
func EnsureAgentCoreRuntimeRole(ctx context.Context, awsCfg aws.Config, accountID, region string, lg Logger) (string, error) {
	lg = loggerOr(lg)
	const roleName = "agentcore-runtime-role"
	const policyName = "AgentCoreRuntimePolicy"

	cli := iam.NewFromConfig(awsCfg)
	if out, err := cli.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)}); err == nil {
		lg.Log(fmt.Sprintf("[awsdeploy] IAM role %s exists", roleName))
		return aws.ToString(out.Role.Arn), nil
	} else {
		var ae smithy.APIError
		if !errors.As(err, &ae) || ae.ErrorCode() != "NoSuchEntity" {
			return "", fmt.Errorf("get role: %w", err)
		}
	}

	trust := mustJSON(map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect":    "Allow",
			"Principal": map[string]any{"Service": "bedrock-agentcore.amazonaws.com"},
			"Action":    "sts:AssumeRole",
			"Condition": map[string]any{"StringEquals": map[string]string{"aws:SourceAccount": accountID}},
		}},
	})
	createOut, err := cli.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trust),
		Description:              aws.String("Shared AgentCore runtime role created by Flow"),
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) && ae.ErrorCode() == "EntityAlreadyExists" {
			// Race: another deploy created it between Get and Create.
			lg.Log(fmt.Sprintf("[awsdeploy] IAM role %s won the race", roleName))
			out, gerr := cli.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
			if gerr != nil {
				return "", gerr
			}
			return aws.ToString(out.Role.Arn), nil
		}
		return "", fmt.Errorf("create role: %w", err)
	}
	roleArn := aws.ToString(createOut.Role.Arn)

	policy := mustJSON(map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{"Effect": "Allow", "Action": []string{"ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer", "ecr:GetAuthorizationToken"}, "Resource": "*"},
			{"Effect": "Allow", "Action": []string{"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents", "logs:DescribeLogStreams", "logs:DescribeLogGroups"}, "Resource": "*"},
			{"Effect": "Allow", "Action": []string{"xray:PutTraceSegments", "xray:PutTelemetryRecords", "xray:GetSamplingRules", "xray:GetSamplingTargets"}, "Resource": "*"},
			{"Effect": "Allow", "Action": "cloudwatch:PutMetricData", "Resource": "*", "Condition": map[string]any{"StringEquals": map[string]string{"cloudwatch:namespace": "bedrock-agentcore"}}},
			{"Effect": "Allow", "Action": []string{"bedrock:InvokeModel", "bedrock:InvokeModelWithResponseStream"}, "Resource": fmt.Sprintf("arn:aws:bedrock:%s::foundation-model/*", region)},
			{"Effect": "Allow", "Action": []string{"bedrock-agentcore:GetWorkloadAccessToken", "bedrock-agentcore:GetWorkloadAccessTokenForJWT", "bedrock-agentcore:GetWorkloadAccessTokenForUserId"}, "Resource": "*"},
		},
	})
	if _, err := cli.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(policy),
	}); err != nil {
		return "", fmt.Errorf("put role policy: %w", err)
	}
	lg.Log(fmt.Sprintf("[awsdeploy] IAM role %s created; sleeping 10s for propagation", roleName))
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(10 * time.Second):
	}
	return roleArn, nil
}

// AgentRuntimeResult holds the post-deploy identifiers returned to the
// Deploy executor's __deploy summary. EndpointArn + InvokeURL are what
// users actually want at the end of the run.
type AgentRuntimeResult struct {
	AgentRuntimeID  string
	AgentRuntimeArn string
	EndpointArn     string
	InvokeURL       string
}

// CreateOrUpdateAgentRuntime puts a new AgentCore runtime (or updates the
// existing one with the same name) and waits for the DEFAULT endpoint to
// reach READY. Mirrors deploy_to_agentcore.js exactly. timeout caps the
// readiness wait; total time also bounded by ctx.
func CreateOrUpdateAgentRuntime(
	ctx context.Context,
	c Config,
	creds aws.CredentialsProvider,
	runtimeName, image, runtimeRoleArn string,
	envVars map[string]string,
	timeout time.Duration,
	lg Logger,
) (AgentRuntimeResult, error) {
	lg = loggerOr(lg)
	if c.Region == "" {
		return AgentRuntimeResult{}, errors.New("region required")
	}
	if image == "" {
		return AgentRuntimeResult{}, errors.New("image required")
	}
	if runtimeRoleArn == "" {
		return AgentRuntimeResult{}, errors.New("runtimeRoleArn required")
	}

	// AgentCore enforces [a-zA-Z0-9_]{1,48}
	agentName := sanitizeAgentName(runtimeName)
	createBody := map[string]any{
		"agentRuntimeName": agentName,
		"agentRuntimeArtifact": map[string]any{
			"containerConfiguration": map[string]any{"containerUri": image},
		},
		"roleArn":              runtimeRoleArn,
		"networkConfiguration": map[string]any{"networkMode": "PUBLIC"},
	}
	if len(envVars) > 0 {
		createBody["environmentVariables"] = envVars
	}

	var (
		agentID  string
		agentArn string
	)

	// Try create.
	respCreate, status, err := agentcoreRequest(ctx, "PUT", "/runtimes/", createBody, c.Region, creds)
	switch {
	case err == nil:
		agentID, _ = respCreate["agentRuntimeId"].(string)
		agentArn, _ = respCreate["agentRuntimeArn"].(string)
		lg.Log(fmt.Sprintf("[awsdeploy] AgentCore created runtime %s", agentID))
	case status == http.StatusConflict || isConflict(err):
		// Update path: list, find by name, update.
		lg.Log(fmt.Sprintf("[awsdeploy] AgentCore runtime %q exists; updating", agentName))
		listResp, _, lerr := agentcoreRequest(ctx, "POST", "/runtimes/", map[string]any{"maxResults": 100}, c.Region, creds)
		if lerr != nil {
			return AgentRuntimeResult{}, fmt.Errorf("list runtimes: %w", lerr)
		}
		runtimes, _ := listResp["agentRuntimes"].([]any)
		for _, r := range runtimes {
			rm, ok := r.(map[string]any)
			if !ok {
				continue
			}
			if rm["agentRuntimeName"] == agentName {
				agentID, _ = rm["agentRuntimeId"].(string)
				agentArn, _ = rm["agentRuntimeArn"].(string)
				break
			}
		}
		if agentID == "" {
			return AgentRuntimeResult{}, fmt.Errorf("conflict but runtime %q not found in list", agentName)
		}
		updateBody := map[string]any{
			"agentRuntimeArtifact": map[string]any{
				"containerConfiguration": map[string]any{"containerUri": image},
			},
			"roleArn":              runtimeRoleArn,
			"networkConfiguration": map[string]any{"networkMode": "PUBLIC"},
		}
		if len(envVars) > 0 {
			updateBody["environmentVariables"] = envVars
		}
		if _, _, uerr := agentcoreRequest(ctx, "PUT", "/runtimes/"+agentID+"/", updateBody, c.Region, creds); uerr != nil {
			return AgentRuntimeResult{}, fmt.Errorf("update runtime: %w", uerr)
		}
		lg.Log(fmt.Sprintf("[awsdeploy] AgentCore updated runtime %s", agentID))
	default:
		return AgentRuntimeResult{}, fmt.Errorf("create runtime: %w", err)
	}

	// Wait for DEFAULT endpoint READY.
	endpointArn, werr := waitEndpointReady(ctx, agentID, c.Region, creds, timeout, lg)
	if werr != nil {
		return AgentRuntimeResult{
			AgentRuntimeID:  agentID,
			AgentRuntimeArn: agentArn,
		}, werr
	}
	invokeURL := fmt.Sprintf("https://bedrock-agentcore.%s.amazonaws.com/runtimes/%s/invocations", c.Region, agentID)
	lg.Log(fmt.Sprintf("[awsdeploy] AgentCore endpoint READY: %s", invokeURL))
	return AgentRuntimeResult{
		AgentRuntimeID:  agentID,
		AgentRuntimeArn: agentArn,
		EndpointArn:     endpointArn,
		InvokeURL:       invokeURL,
	}, nil
}

func waitEndpointReady(ctx context.Context, agentID, region string, creds aws.CredentialsProvider, timeout time.Duration, lg Logger) (string, error) {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	deadline := time.Now().Add(timeout)
	path := fmt.Sprintf("/runtimes/%s/runtime-endpoints/DEFAULT/", agentID)
	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("endpoint not READY within %s", timeout)
		}
		resp, status, err := agentcoreRequest(ctx, "GET", path, nil, region, creds)
		if err != nil {
			if status == http.StatusNotFound {
				lg.Log("[awsdeploy] endpoint not provisioned yet, waiting...")
			} else {
				return "", err
			}
		} else {
			s, _ := resp["status"].(string)
			lg.Log(fmt.Sprintf("[awsdeploy] endpoint status: %s", s))
			if s == "READY" {
				arn, _ := resp["agentRuntimeEndpointArn"].(string)
				return arn, nil
			}
			if strings.Contains(s, "FAILED") {
				reason, _ := resp["failureReason"].(string)
				return "", fmt.Errorf("endpoint failed: %s (%s)", s, reason)
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// agentcoreRequest signs and sends a request to bedrock-agentcore-control.
// Returns the parsed JSON response, HTTP status, and a non-nil error if
// status >= 400 or transport failed.
func agentcoreRequest(
	ctx context.Context,
	method, path string,
	body map[string]any,
	region string,
	creds aws.CredentialsProvider,
) (map[string]any, int, error) {
	host := fmt.Sprintf("bedrock-agentcore-control.%s.amazonaws.com", region)
	endpoint := url.URL{Scheme: "https", Host: host, Path: path}

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("content-type", "application/json")
	req.Host = host

	c, err := creds.Retrieve(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("retrieve creds: %w", err)
	}
	hash := sha256Hex(bodyBytes)
	signer := v4.NewSigner()
	if err := signer.SignHTTP(ctx, c, req, hash, "bedrock-agentcore", region, time.Now()); err != nil {
		return nil, 0, fmt.Errorf("sigv4 sign: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var parsed map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &parsed)
	}
	if resp.StatusCode >= 400 {
		msg := ""
		if parsed != nil {
			if m, ok := parsed["message"].(string); ok {
				msg = m
			} else if m, ok := parsed["Message"].(string); ok {
				msg = m
			}
		}
		if msg == "" {
			msg = string(raw)
		}
		return parsed, resp.StatusCode, fmt.Errorf("agentcore HTTP %d: %s", resp.StatusCode, msg)
	}
	return parsed, resp.StatusCode, nil
}

func isConflict(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "conflict")
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func sanitizeAgentName(in string) string {
	var b strings.Builder
	for _, r := range in {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
