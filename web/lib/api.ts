// API client for the Go flow server. Static export → calls go to same origin.

export type FlowSummary = {
  id: string;
  name: string;
  description?: string;
  updatedAt: string;
  nodeCount: number;
  status?: string;
};

export type StoredFlow = {
  id: string;
  name: string;
  definition: unknown;
  updatedAt: string;
  nodeCount: number;
};

export type ExecutionStatus = {
  execution_id?: string;
  status?: string;
  [k: string]: unknown;
};

export type AuthStatus = "untested" | "ok" | "failed";

export type CredentialType = "aws" | "gcp" | "kv";

export type PublicCredential = {
  id: string;
  name: string;
  type: CredentialType;
  createdAt: string;
  updatedAt: string;
  awsRegion?: string;
  awsAccountId?: string;
  awsCrossAccountRoleArn?: string;
  gcpProjectId?: string;
  gcpLocation?: string;
  hasServiceAccount?: boolean;
  kvKeys?: string[];
};

export type CredentialBody = {
  name: string;
  type: CredentialType;
  awsRegion?: string;
  awsAccountId?: string;
  awsCrossAccountRoleArn?: string;
  gcpProjectId?: string;
  gcpLocation?: string;
  gcpServiceAccountJson?: string;
  kv?: Record<string, string>;
};

export type Agent = {
  id: string;
  name: string;
  repoUrl: string;
  ref?: string;
  hasPat: boolean;
  webhookId?: number;
  webhookUrl?: string;
  webhookInstalled: boolean;
  webhookInstalledAt?: string;
  authStatus?: AuthStatus;
  authCheckedAt?: string;
  attachedPipelines?: string[];
  credentials?: PublicCredential[];
  createdAt: string;
  updatedAt: string;
};

export type Run = {
  id: string;
  pipelineId: string;
  pipelineName?: string;
  status: string;
  startedAt: string;
  finishedAt?: string;
  triggerData?: unknown;
  outputs?: unknown;
  nodeOutputs?: unknown;
  errors?: string[];
};

export type ServerConfig = {
  publicUrl: string;
  webhooksAvailable: boolean;
  orchestratorEnabled: boolean;
};

// REST + page calls go same-origin (Next rewrite proxies /api → Go in dev,
// nginx proxies /api → flow:8090 in prod).
const base = "";

// SSE base for streaming endpoints.
//
// Default: same-origin (works behind any reverse proxy that doesn't buffer
// — nginx with `proxy_buffering off`, our prod config; Cloudflare tunnels;
// most production setups).
//
// Dev override: set NEXT_PUBLIC_FLOW_API_URL=http://localhost:8090 to hit
// the Go server directly, bypassing Next's dev rewrite (which buffers
// chunked responses, breaking node-by-node updates) and Next's 308 redirect
// from `trailingSlash: true` (which EventSource won't follow).
const sseBase =
  (typeof process !== "undefined" &&
    process.env?.NEXT_PUBLIC_FLOW_API_URL) ||
  "";

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  health: () => fetch(`${base}/api/health`).then(handle<{ status: string }>),

  listFlows: () => fetch(`${base}/api/workflows`).then(handle<FlowSummary[]>),

  getFlow: (id: string) => fetch(`${base}/api/workflows/${id}`).then(handle<StoredFlow>),

  createFlow: (body: { name: string; definition: unknown }) =>
    fetch(`${base}/api/workflows`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<{ id: string }>),

  updateFlow: (id: string, body: { name?: string; definition?: unknown }) =>
    fetch(`${base}/api/workflows/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<void>),

  deleteFlow: (id: string) =>
    fetch(`${base}/api/workflows/${id}`, { method: "DELETE" }).then(handle<void>),

  executeWorkflow: (body: { workflow_id?: string; workflow?: unknown; input?: unknown[] }) =>
    fetch(`${base}/api/workflows/execute`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<{ execution_id: string; status: string }>),

  getExecution: (id: string) =>
    fetch(`${base}/api/executions/${id}`).then(handle<ExecutionStatus>),

  resumeExecution: (id: string, body: { awakeable_id: string; data?: unknown }) =>
    fetch(`${base}/api/executions/${id}/resume`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<{ message: string }>),

  // --- config ---
  getConfig: () => fetch(`${base}/api/config`).then(handle<ServerConfig>),

  // --- agents ---
  listAgents: () => fetch(`${base}/api/agents`).then(handle<Agent[]>),

  createAgent: (body: { repoUrl: string; pat?: string; ref?: string; name?: string }) =>
    fetch(`${base}/api/agents`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<Agent>),

  getAgent: (id: string) =>
    fetch(`${base}/api/agents/${id}`).then(handle<Agent>),

  deleteAgent: (id: string) =>
    fetch(`${base}/api/agents/${id}`, { method: "DELETE" }).then(handle<void>),

  testAgentAuth: (id: string) =>
    fetch(`${base}/api/agents/${id}/test-auth`, { method: "POST" }).then(
      handle<{ authStatus: AuthStatus; authCheckedAt: string; error?: string }>
    ),

  installAgentWebhook: (id: string) =>
    fetch(`${base}/api/agents/${id}/webhook`, { method: "POST" }).then(handle<Agent>),

  uninstallAgentWebhook: (id: string) =>
    fetch(`${base}/api/agents/${id}/webhook`, { method: "DELETE" }).then(handle<Agent>),

  attachPipeline: (id: string, pipelineId: string) =>
    fetch(`${base}/api/agents/${id}/pipelines/${pipelineId}`, {
      method: "POST",
    }).then(handle<Agent>),

  detachPipeline: (id: string, pipelineId: string) =>
    fetch(`${base}/api/agents/${id}/pipelines/${pipelineId}`, {
      method: "DELETE",
    }).then(handle<void>),

  triggerAgent: (id: string) =>
    fetch(`${base}/api/agents/${id}/trigger`, { method: "POST" }).then(
      handle<{
        executionIds: string[];
        failures?: { pipelineId: string; reason: string; error?: string }[];
      }>
    ),

  listCredentials: (agentId: string) =>
    fetch(`${base}/api/agents/${agentId}/credentials`).then(
      handle<PublicCredential[]>
    ),

  listGlobalCredentials: () =>
    fetch(`${base}/api/credentials`).then(handle<PublicCredential[]>),

  getGlobalCredential: (name: string) =>
    fetch(`${base}/api/credentials/${encodeURIComponent(name)}`).then(
      handle<PublicCredential>
    ),

  createGlobalCredential: (body: CredentialBody) =>
    fetch(`${base}/api/credentials`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<PublicCredential>),

  updateGlobalCredential: (name: string, body: CredentialBody) =>
    fetch(`${base}/api/credentials/${encodeURIComponent(name)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<PublicCredential>),

  deleteGlobalCredential: (name: string) =>
    fetch(`${base}/api/credentials/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }).then(handle<void>),

  createCredential: (agentId: string, body: CredentialBody) =>
    fetch(`${base}/api/agents/${agentId}/credentials`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<PublicCredential>),

  updateCredential: (agentId: string, name: string, body: CredentialBody) =>
    fetch(`${base}/api/agents/${agentId}/credentials/${encodeURIComponent(name)}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }).then(handle<PublicCredential>),

  deleteCredential: (agentId: string, name: string) =>
    fetch(`${base}/api/agents/${agentId}/credentials/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }).then(handle<void>),

  // --- runs ---
  /** Returns the EventSource URL for SSE streaming of an execution.
   *  Uses `sseBase` so dev can hit the Go API directly (skipping Next's
   *  trailingSlash 308 which EventSource won't follow). Trailing slash on
   *  the path keeps things consistent if the user does proxy through Next
   *  or nginx; the Go mux registers both forms either way. */
  executionStreamURL: (id: string) => `${sseBase}/api/executions/${id}/stream/`,

  /** Global runs feed — fires once per dispatched run. Same dev-bypass
   *  reasoning as executionStreamURL. */
  runsStreamURL: () => `${sseBase}/api/runs/stream/`,

  listRuns: (params?: { pipelineId?: string; limit?: number }) => {
    const qs = new URLSearchParams();
    if (params?.pipelineId) qs.set("pipeline_id", params.pipelineId);
    if (params?.limit) qs.set("limit", String(params.limit));
    const q = qs.toString();
    return fetch(`${base}/api/executions${q ? `?${q}` : ""}`).then(handle<Run[]>);
  },
};
