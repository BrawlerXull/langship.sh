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
};
