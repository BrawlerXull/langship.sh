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

const base = ""; // same-origin

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

  // --- runs ---
  /** Returns the EventSource URL for SSE streaming of an execution. */
  executionStreamURL: (id: string) => `${base}/api/executions/${id}/stream`,

  listRuns: (params?: { pipelineId?: string; limit?: number }) => {
    const qs = new URLSearchParams();
    if (params?.pipelineId) qs.set("pipeline_id", params.pipelineId);
    if (params?.limit) qs.set("limit", String(params.limit));
    const q = qs.toString();
    return fetch(`${base}/api/executions${q ? `?${q}` : ""}`).then(handle<Run[]>);
  },
};
