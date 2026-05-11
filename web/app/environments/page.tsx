"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowDown, ArrowUp, Layers, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { api, type Environment, type FlowSummary } from "@/lib/api";

export default function EnvironmentsPage() {
  const [envs, setEnvs] = useState<Environment[] | null>(null);
  const [pipelines, setPipelines] = useState<FlowSummary[]>([]);
  const [error, setError] = useState<string | null>(null);

  async function refresh() {
    setError(null);
    try {
      const [e, p] = await Promise.all([api.listEnvironments(), api.listFlows()]);
      setEnvs(e);
      setPipelines(p);
    } catch (err) {
      setError(err instanceof Error ? err.message : "load failed");
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function handleDelete(name: string) {
    if (!confirm(`Delete environment "${name}"? Agents following it will stop dispatching its pipelines.`)) return;
    try {
      await api.deleteEnvironment(name);
      await refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  const pipelineName = (id: string) =>
    pipelines.find((p) => p.id === id)?.name ?? id;

  return (
    <div className="w-full space-y-6 p-6">
      <div className="flex items-start justify-between gap-3">
        <div>
          <div className="mb-1 flex items-center gap-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
            <Layers className="size-3.5" />
            Environments
          </div>
          <h1 className="text-3xl font-semibold tracking-tight">Environments</h1>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            A deploy stage is a named, ordered list of pipelines — the
            promotion sequence. Agents <em>follow</em> environments;
            triggering an agent runs its followed envs&rsquo; pipelines (each
            still gated by its Trigger node&rsquo;s branch).
          </p>
        </div>
        <Button size="sm" asChild>
          <Link href="/environments/new">
            <Plus />
            New environment
          </Link>
        </Button>
      </div>

      {error && (
        <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
          {error}
        </p>
      )}

      {envs === null && <p className="text-sm text-muted-foreground">Loading…</p>}
      {envs?.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No environments yet. Create <code className="font-mono">dev</code>,{" "}
          <code className="font-mono">staging</code>, and{" "}
          <code className="font-mono">prod</code> to get started.
        </p>
      )}

      <div className="space-y-4">
        {envs?.map((env) => (
          <Card key={env.id}>
            <CardHeader className="flex flex-row items-start justify-between gap-2 space-y-0">
              <div>
                <CardTitle className="font-mono">{env.name}</CardTitle>
                {env.description && <CardDescription>{env.description}</CardDescription>}
              </div>
              <div className="flex shrink-0 gap-2">
                <Button size="sm" variant="ghost" asChild>
                  <Link href={`/environments/edit/?name=${encodeURIComponent(env.name)}`}>
                    Edit
                  </Link>
                </Button>
                <Button size="sm" variant="ghost" onClick={() => handleDelete(env.name)}>
                  Delete
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <PipelinesInEnv
                env={env}
                allPipelines={pipelines}
                pipelineName={pipelineName}
                onChanged={refresh}
                onError={setError}
              />
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

// ─── pipelines-in-env sub-component ─────────────────────────────────────────

function PipelinesInEnv({
  env,
  allPipelines,
  pipelineName,
  onChanged,
  onError,
}: {
  env: Environment;
  allPipelines: FlowSummary[];
  pipelineName: (id: string) => string;
  onChanged: () => void | Promise<void>;
  onError: (m: string | null) => void;
}) {
  const [picking, setPicking] = useState(false);
  const ids = env.pipelineIds ?? [];
  const inEnv = new Set(ids);
  const available = allPipelines.filter((p) => !inEnv.has(p.id));

  async function reorder(next: string[]) {
    try {
      await api.reorderEnvPipelines(env.name, next);
      await onChanged();
    } catch (e) {
      onError(e instanceof Error ? e.message : "reorder failed");
    }
  }
  function move(i: number, dir: -1 | 1) {
    const j = i + dir;
    if (j < 0 || j >= ids.length) return;
    const next = ids.slice();
    [next[i], next[j]] = [next[j], next[i]];
    reorder(next);
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <div className="text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          Pipelines in this environment (promotion order)
        </div>
        <Button size="sm" variant="ghost" onClick={() => setPicking((v) => !v)}>
          <Plus className="size-3.5" />
          Add pipeline
        </Button>
      </div>

      {ids.length === 0 && (
        <p className="text-sm text-muted-foreground">
          No pipelines yet. Build one on the{" "}
          <Link href="/" className="underline">Pipelines page</Link> and add it
          here.
        </p>
      )}

      {ids.length > 0 && (
        <ul className="divide-y rounded-md border">
          {ids.map((pid, i) => (
            <li key={pid} className="flex items-center gap-2 px-3 py-2">
              <span className="w-5 shrink-0 text-center text-xs text-muted-foreground">
                {i + 1}
              </span>
              <Link
                href={`/flows/edit/?id=${encodeURIComponent(pid)}`}
                className="flex-1 truncate font-mono text-xs hover:underline"
              >
                {pipelineName(pid)}
              </Link>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  size="icon"
                  variant="ghost"
                  className="size-7"
                  disabled={i === 0}
                  onClick={() => move(i, -1)}
                  aria-label="Move up"
                >
                  <ArrowUp className="size-3.5" />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  className="size-7"
                  disabled={i === ids.length - 1}
                  onClick={() => move(i, 1)}
                  aria-label="Move down"
                >
                  <ArrowDown className="size-3.5" />
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={async () => {
                    try {
                      await api.envRemovePipeline(env.name, pid);
                      await onChanged();
                    } catch (e) {
                      onError(e instanceof Error ? e.message : "remove failed");
                    }
                  }}
                >
                  Remove
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {picking && (
        <div className="rounded-md border bg-muted/20 p-2">
          {available.length === 0 ? (
            <p className="px-1 py-1 text-xs text-muted-foreground">
              All pipelines are already in this environment.
            </p>
          ) : (
            <ul className="divide-y">
              {available.map((p) => (
                <li key={p.id} className="flex items-center justify-between px-1 py-1.5">
                  <span className="font-mono text-xs">{p.name}</span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={async () => {
                      try {
                        await api.envAddPipeline(env.name, p.id);
                        setPicking(false);
                        await onChanged();
                      } catch (e) {
                        onError(e instanceof Error ? e.message : "add failed");
                      }
                    }}
                  >
                    Add
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
