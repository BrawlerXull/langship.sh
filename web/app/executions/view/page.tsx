"use client";

import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ChevronDown, ChevronRight, RefreshCw, Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { PipelineCanvas } from "@/components/canvas/pipeline-canvas";
import { api, type ExecutionStatus, type Run } from "@/lib/api";
import type { PipelineDefinition, PipelineNode } from "@/lib/pipeline-graph";
import { lookup as lookupNode } from "@/lib/node-catalog";
import { formatDate } from "@/lib/utils";

type NodeStatus = "pending" | "running" | "success" | "failed" | "paused";
type NodeStatuses = Record<string, NodeStatus>;
type NodeLogs = Record<string, string[]>;
type NodeDurations = Record<string, number>;

interface ExecutionEvent {
  type: string;
  node?: string;
  node_type?: string;
  status?: string;
  content?: string;
  outputs?: Record<string, unknown>;
  error?: string;
  duration_ms?: number;
}

export default function ExecutionPage() {
  return (
    <Suspense
      fallback={<div className="p-6 text-sm text-muted-foreground">Loading…</div>}
    >
      <ExecutionView />
    </Suspense>
  );
}

function statusVariant(s?: string) {
  switch ((s || "").toLowerCase()) {
    case "success":
    case "completed":
      return "success" as const;
    case "running":
    case "pending":
      return "secondary" as const;
    case "failed":
    case "error":
      return "destructive" as const;
    case "waiting":
    case "paused":
      return "warning" as const;
    default:
      return "outline" as const;
  }
}

function ExecutionView() {
  const params = useSearchParams();
  const id = params.get("id") ?? "";

  const [status, setStatus] = useState<ExecutionStatus | null>(null);
  const [run, setRun] = useState<Run | null>(null);
  const [pipelineDef, setPipelineDef] = useState<PipelineDefinition | null>(null);
  const [nodeStatuses, setNodeStatuses] = useState<NodeStatuses>({});
  // Per-node tick when we first marked it running. Used to enforce a
  // minimum visible "running" duration so the user always sees the
  // spinner — even for instantaneous nodes (Trigger, NoOp). Without
  // this, fast nodes flicker pending → success in one render batch and
  // the running state is invisible.
  const runningSinceRef = useRef<Record<string, number>>({});
  const [nodeLogs, setNodeLogs] = useState<NodeLogs>({});
  const [nodeDurations, setNodeDurations] = useState<NodeDurations>({});
  const [streamConnected, setStreamConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Resume form
  const [awakeable, setAwakeable] = useState("");
  const [data, setData] = useState(`{"approved": true}`);
  const [resuming, setResuming] = useState(false);

  // Initial load
  useEffect(() => {
    if (!id) return;
    (async () => {
      try {
        const [statusRes, runs] = await Promise.all([
          api.getExecution(id).catch(() => null),
          api.listRuns({ limit: 100 }).catch(() => [] as Run[]),
        ]);
        if (statusRes) setStatus(statusRes);
        const r = runs.find((x) => x.id === id) ?? null;
        setRun(r);
        if (r?.pipelineId) {
          try {
            const f = await api.getFlow(r.pipelineId);
            setPipelineDef((f.definition as PipelineDefinition | null) ?? null);
          } catch {
            /* pipeline may have been deleted */
          }
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "load failed");
      }
    })();
  }, [id]);

  // SSE
  useEffect(() => {
    if (!id) return;
    const es = new EventSource(api.executionStreamURL(id));
    es.onopen = () => setStreamConnected(true);
    es.onerror = () => setStreamConnected(false);
    es.onmessage = (msg) => {
      try {
        const ev: ExecutionEvent = JSON.parse(msg.data);
        if (ev.type === "node_log" && ev.node && typeof ev.content === "string") {
          const node = ev.node;
          setNodeLogs((prev) => {
            const cur = prev[node] ?? [];
            const next = cur.length >= 2000 ? cur.slice(-1999) : cur;
            return { ...prev, [node]: [...next, ev.content!] };
          });
          return;
        }
        if (ev.node) {
          const node = ev.node;
          const minVisibleMs = 400;

          if (ev.type === "node_started") {
            runningSinceRef.current[node] = Date.now();
            setNodeStatuses((prev) => ({ ...prev, [node]: "running" }));
          } else if (ev.type === "node_completed" || ev.type === "node_error") {
            const final: NodeStatus =
              ev.type === "node_completed" ? "success" : "failed";
            const startedAt = runningSinceRef.current[node];
            const elapsed = startedAt ? Date.now() - startedAt : Infinity;

            // Make sure the user actually sees a "running" frame. If we
            // never recorded a start (subscriber arrived after the start
            // event flushed) we apply the terminal state immediately.
            if (startedAt === undefined || elapsed >= minVisibleMs) {
              setNodeStatuses((prev) => ({ ...prev, [node]: final }));
            } else {
              // Briefly show "running" first if we missed it, then flip.
              setNodeStatuses((prev) => ({
                ...prev,
                [node]: prev[node] === "running" ? "running" : "running",
              }));
              setTimeout(() => {
                setNodeStatuses((prev) => ({ ...prev, [node]: final }));
              }, minVisibleMs - elapsed);
            }
            delete runningSinceRef.current[node];

            if (typeof ev.duration_ms === "number") {
              setNodeDurations((prev) => ({
                ...prev,
                [node]: ev.duration_ms!,
              }));
            }
          }
        }
        if (ev.type === "done") {
          api.getExecution(id).then(setStatus).catch(() => {});
          es.close();
          setStreamConnected(false);
        }
      } catch {
        /* ignore */
      }
    };
    return () => {
      es.close();
      setStreamConnected(false);
    };
  }, [id]);

  // Fallback polling when SSE drops
  useEffect(() => {
    if (!id || streamConnected) return;
    const t = setInterval(() => {
      api.getExecution(id).then(setStatus).catch(() => {});
    }, 4000);
    return () => clearInterval(t);
  }, [id, streamConnected]);

  // Hydrate canvas node statuses from the terminal payload whenever we
  // have status + pipeline def. This handles the "joined after the run
  // finished" case — without it, the canvas stays at PENDING forever
  // because SSE has nothing left to replay. SSE-derived statuses are not
  // overwritten so an in-flight run is still authoritative.
  useEffect(() => {
    if (!status || !pipelineDef) return;
    const overall = String(status.status || "").toLowerCase();
    const isTerm = ["success", "completed", "failed", "error", "partial_error"].includes(overall);
    if (!isTerm) return;
    setNodeStatuses((prev) => {
      const next: NodeStatuses = { ...prev };
      for (const n of pipelineDef.nodes ?? []) {
        if (next[n.name]) continue; // SSE wins
        next[n.name] = inferStatus(status, n.name);
      }
      return next;
    });
  }, [status, pipelineDef]);

  async function onResume() {
    setResuming(true);
    setError(null);
    try {
      let payload: unknown = {};
      if (data.trim()) payload = JSON.parse(data);
      await api.resumeExecution(id, { awakeable_id: awakeable, data: payload });
      const s = await api.getExecution(id).catch(() => null);
      if (s) setStatus(s);
    } catch (e) {
      setError(e instanceof Error ? e.message : "resume failed");
    } finally {
      setResuming(false);
    }
  }

  const overallStatus = useMemo(
    () => (status?.status as string) || run?.status || "running",
    [status, run]
  );
  const isTerminal = ["success", "completed", "failed", "error", "partial_error"]
    .includes(overallStatus.toLowerCase());

  const nodes: PipelineNode[] = pipelineDef?.nodes ?? [];

  if (!id) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        Missing <code>id</code> query param.
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6 pb-24">
      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Run</h1>
          <div className="mt-1 font-mono text-xs text-muted-foreground">{id}</div>
          {run?.pipelineId && (
            <Link
              href={`/flows/view/?id=${encodeURIComponent(run.pipelineId)}`}
              className="mt-1 inline-block text-sm underline-offset-4 hover:underline"
            >
              pipeline {run.pipelineId.slice(0, 8)}
            </Link>
          )}
        </div>
        <Badge variant={statusVariant(overallStatus)} className="gap-1">
          <span
            className={
              "size-1.5 rounded-full " +
              (overallStatus === "running"
                ? "bg-sky-500 animate-pulse"
                : overallStatus === "success" || overallStatus === "completed"
                  ? "bg-emerald-500"
                  : overallStatus === "failed" || overallStatus === "error"
                    ? "bg-rose-500"
                    : "bg-muted-foreground")
            }
          />
          {overallStatus}
        </Badge>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {/* Pipeline canvas card */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle>Pipeline</CardTitle>
          <span className="text-xs text-muted-foreground">
            {nodes.length} {nodes.length === 1 ? "node" : "nodes"}
          </span>
        </CardHeader>
        <CardContent>
          <div className="relative h-[440px] overflow-hidden rounded-lg border bg-background">
            {pipelineDef ? (
              <PipelineCanvas
                initialValue={pipelineDef}
                pipelineId={`exec-${id}`}
                readOnly
                fullBleed
                nodeStatuses={nodeStatuses}
              />
            ) : (
              <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                {run?.pipelineId
                  ? "Loading pipeline canvas…"
                  : "No pipeline definition for this run."}
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Nodes panel */}
      <Card>
        <CardHeader>
          <CardTitle>Nodes</CardTitle>
          <CardDescription>
            Per-node detail. Build logs stream live and archive to S3 when the
            node finishes.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {nodes.length === 0 ? (
            <p className="text-sm text-muted-foreground">No nodes loaded.</p>
          ) : (
            nodes.map((n) => (
              <NodeRow
                key={n.id}
                node={n}
                status={nodeStatuses[n.name] ?? (isTerminal ? inferStatus(status, n.name) : "pending")}
                durationMs={nodeDurations[n.name]}
                liveLog={nodeLogs[n.name]}
                streaming={streamConnected && nodeStatuses[n.name] === "running"}
                isTerminal={isTerminal}
                executionId={id}
                statusOutputs={status?.node_outputs as Record<string, unknown> | undefined}
              />
            ))
          )}
        </CardContent>
      </Card>

      {/* Resume overlay if paused */}
      {(overallStatus === "paused" || overallStatus === "waiting") && (
        <Card className="fixed bottom-6 right-6 z-30 w-80 shadow-xl">
          <CardHeader>
            <CardTitle>Resume</CardTitle>
            <CardDescription>
              Resolve a Restate awakeable to continue.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="space-y-1.5">
              <Label htmlFor="awakeable">Awakeable ID</Label>
              <Input
                id="awakeable"
                value={awakeable}
                onChange={(e) => setAwakeable(e.target.value)}
                placeholder="awk_…"
                className="font-mono text-xs"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="data">Resolution data (JSON)</Label>
              <Textarea
                id="data"
                rows={4}
                value={data}
                onChange={(e) => setData(e.target.value)}
                spellCheck={false}
                className="text-xs"
              />
            </div>
            <Button
              className="w-full"
              onClick={onResume}
              disabled={resuming || !awakeable}
            >
              <Send />
              {resuming ? "Sending…" : "Resume"}
            </Button>
          </CardContent>
        </Card>
      )}

      <div className="flex justify-end">
        <Button
          variant="outline"
          size="sm"
          onClick={() => api.getExecution(id).then(setStatus).catch(() => {})}
        >
          <RefreshCw />
          Refresh
        </Button>
      </div>
    </div>
  );
}

// inferStatus reads the orchestrator's terminal payload to decide whether a
// node ran. Used after `done` when the SSE map may be incomplete (e.g. user
// landed on the page after the run finished).
function inferStatus(
  status: ExecutionStatus | null,
  nodeName: string
): NodeStatus {
  if (!status) return "pending";
  const outs = status.node_outputs as Record<string, unknown> | undefined;
  if (outs && nodeName in outs) return "success";
  const errs = (status.errors as string[] | undefined) ?? [];
  for (const e of errs) {
    if (typeof e === "string" && e.includes(`node "${nodeName}"`)) return "failed";
  }
  return "pending";
}

function NodeRow(props: {
  node: PipelineNode;
  status: NodeStatus;
  durationMs?: number;
  liveLog?: string[];
  streaming: boolean;
  isTerminal: boolean;
  executionId: string;
  statusOutputs?: Record<string, unknown>;
}) {
  const { node, status, durationMs, liveLog, streaming, isTerminal, executionId, statusOutputs } = props;
  const entry = lookupNode(node.type);
  const typeLabel = entry?.label?.toLowerCase() ?? node.type.replace("flow-nodes-base.", "");

  const buildSummary = extractBuildSummary(node.name, statusOutputs);
  const triggerSummary = extractTriggerSummary(node.name, statusOutputs);

  return (
    <div className="rounded-lg border bg-muted/10">
      <div className="flex items-center justify-between gap-2 p-4">
        <div className="min-w-0">
          <div className="text-base font-medium">{node.name}</div>
          <div className="text-xs text-muted-foreground">{typeLabel}</div>
        </div>
        <div className="flex items-center gap-3 text-xs text-muted-foreground">
          {typeof durationMs === "number" && (
            <span>took {(durationMs / 1000).toFixed(1)}s</span>
          )}
          <NodeStatusDot status={status} />
        </div>
      </div>

      {/* Build node — image / digest / log path */}
      {buildSummary && (
        <div className="border-t px-4 py-3 font-mono text-[11px] leading-6">
          {buildSummary.image && (
            <div>
              <span className="text-muted-foreground">image: </span>
              <span>{buildSummary.image}</span>
            </div>
          )}
          {buildSummary.digest && (
            <div>
              <span className="text-muted-foreground">digest: </span>
              <span className="break-all">{buildSummary.digest}</span>
            </div>
          )}
          {buildSummary.commit && (
            <div>
              <span className="text-muted-foreground">sha: </span>
              <span>{buildSummary.commit}</span>
            </div>
          )}
          {buildSummary.command && (
            <div>
              <span className="text-muted-foreground">command: </span>
              <span>{buildSummary.command}</span>
            </div>
          )}
          {/* Always render the log path so the field appears even when archived. */}
          <div>
            <span className="text-muted-foreground">log: </span>
            <span>s3://flow-logs/runs/{executionId}/{node.name}.log</span>
          </div>
        </div>
      )}

      {triggerSummary && (
        <div className="border-t px-4 py-3 font-mono text-[11px] leading-6">
          <div>
            <span className="text-muted-foreground">source: </span>
            <span>{triggerSummary.source ?? "manual"}</span>
          </div>
          {triggerSummary.repoUrl && (
            <div>
              <span className="text-muted-foreground">repo: </span>
              <span>{triggerSummary.repoUrl}</span>
            </div>
          )}
          {triggerSummary.ref && (
            <div>
              <span className="text-muted-foreground">ref: </span>
              <span>{triggerSummary.ref}</span>
            </div>
          )}
        </div>
      )}

      {/* Build log disclosure — visible for all node types that emitted lines */}
      {(liveLog?.length || isTerminal) && (
        <LogDisclosure
          executionId={executionId}
          node={node}
          liveLog={liveLog}
          streaming={streaming}
          isTerminal={isTerminal}
          status={status}
          fallbackLog={buildSummary?.logTail}
        />
      )}
    </div>
  );
}

function NodeStatusDot({ status }: { status: NodeStatus }) {
  if (status === "running") {
    return (
      <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-500">
        <span className="size-1.5 rounded-full bg-sky-500 animate-pulse" />
        running
      </span>
    );
  }
  if (status === "success") {
    return (
      <span className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-500">
        <span className="size-1.5 rounded-full bg-emerald-500" />
        succeeded
      </span>
    );
  }
  if (status === "failed") {
    return (
      <span className="inline-flex items-center gap-1 text-rose-600 dark:text-rose-500">
        <span className="size-1.5 rounded-full bg-rose-500" />
        failed
      </span>
    );
  }
  if (status === "paused") {
    return (
      <span className="inline-flex items-center gap-1 text-amber-600 dark:text-amber-500">
        <span className="size-1.5 rounded-full bg-amber-500" />
        paused
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-muted-foreground">
      <span className="size-1.5 rounded-full bg-muted-foreground/60" />
      pending
    </span>
  );
}

function LogDisclosure(props: {
  executionId: string;
  node: PipelineNode;
  liveLog?: string[];
  streaming: boolean;
  isTerminal: boolean;
  status: NodeStatus;
  /** Fallback log content from the orchestrator's terminal payload
   *  (e.g. `__build.log_tail`). Used when the archive isn't configured
   *  and we joined the page after the run finished. */
  fallbackLog?: string;
}) {
  const { executionId, node, liveLog, streaming, isTerminal, status, fallbackLog } = props;
  const [open, setOpen] = useState(false);
  const [archived, setArchived] = useState<string | null>(null);
  const [archiveErr, setArchiveErr] = useState<string | null>(null);
  const fetched = useRef(false);

  const live = liveLog ?? [];
  // Show archived log when: run is terminal, no live lines this session,
  // and the archive endpoint returns 200.
  const tryArchive = isTerminal && live.length === 0 && (status === "success" || status === "failed");

  useEffect(() => {
    if (!open || !tryArchive || fetched.current) return;
    fetched.current = true;
    (async () => {
      try {
        const res = await fetch(
          `/api/executions/${executionId}/logs/${encodeURIComponent(node.name)}`
        );
        if (!res.ok) {
          setArchiveErr(`archive ${res.status}`);
          return;
        }
        setArchived(await res.text());
      } catch (e) {
        setArchiveErr(e instanceof Error ? e.message : "fetch failed");
      }
    })();
  }, [open, tryArchive, executionId, node.name]);

  // Pick the best content source available, in priority order:
  //   1. live SSE buffer (this session)
  //   2. archived object (MinIO, if configured)
  //   3. fallbackLog (last 80 lines from log_tail in node_outputs)
  const usingArchive = archived !== null;
  const usingFallback = !usingArchive && live.length === 0 && Boolean(fallbackLog);

  const labelStr = streaming
    ? "live"
    : live.length > 0
      ? "live (cached)"
      : usingArchive
        ? "archived"
        : usingFallback
          ? "tail"
          : tryArchive && archiveErr
            ? `unarchived (${archiveErr})`
            : "no output yet";

  return (
    <div className="border-t">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between px-4 py-2 text-left text-xs font-medium hover:bg-muted/30"
      >
        <span className="flex items-center gap-1.5 text-muted-foreground">
          {open ? (
            <ChevronDown className="size-3.5" />
          ) : (
            <ChevronRight className="size-3.5" />
          )}
          build log
        </span>
        <span className="text-[10px] uppercase tracking-wider text-muted-foreground">
          {labelStr}
        </span>
      </button>
      {open && (
        <div className="border-t bg-zinc-950 px-3 py-2 font-mono text-[11px] leading-[1.45] text-zinc-200">
          {live.length > 0 ? (
            <div className="max-h-80 overflow-auto">
              {live.map((l, i) => (
                <div key={i} className="whitespace-pre-wrap break-all">
                  {l}
                </div>
              ))}
            </div>
          ) : usingArchive ? (
            <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all text-zinc-200">
              {archived}
            </pre>
          ) : tryArchive && archived === null && !archiveErr ? (
            <div className="text-zinc-500">loading archive…</div>
          ) : usingFallback ? (
            <>
              <div className="mb-2 text-[10px] uppercase tracking-wider text-amber-400">
                Showing the last lines from the orchestrator (log archive
                {archiveErr ? ` ${archiveErr}` : " not configured"}). Set
                MINIO_ENDPOINT on the flow service to enable full archive.
              </div>
              <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all text-zinc-200">
                {fallbackLog}
              </pre>
            </>
          ) : (
            <div className="text-zinc-500">waiting for output…</div>
          )}
        </div>
      )}
    </div>
  );
}

// Pull image / digest / commit / command from a Build node's output items
// in the orchestrator's terminal node_outputs payload.
function extractBuildSummary(
  nodeName: string,
  outputs?: Record<string, unknown>
): { image?: string; digest?: string; commit?: string; command?: string; logTail?: string } | null {
  if (!outputs) return null;
  const slot = outputs[nodeName];
  if (!slot || typeof slot !== "object") return null;
  // node_outputs[name] = {0: [items]}
  const portMap = slot as Record<string, unknown>;
  const port = portMap["0"];
  if (!Array.isArray(port) || port.length === 0) return null;
  const item = port[0] as Record<string, unknown>;
  const build = item["__build"] as Record<string, unknown> | undefined;
  if (!build) return null;
  const out: {
    image?: string;
    digest?: string;
    commit?: string;
    command?: string;
    logTail?: string;
  } = {};
  if (typeof build.image === "string") out.image = build.image;
  if (typeof build.commit === "string") out.commit = build.commit;
  if (typeof build.command === "string") out.command = build.command;
  if (typeof build.log_tail === "string") {
    out.logTail = build.log_tail;
    const m = build.log_tail.match(/sha256:[0-9a-f]{64}/);
    if (m) out.digest = m[0];
  }
  return out;
}

function extractTriggerSummary(
  nodeName: string,
  outputs?: Record<string, unknown>
): { source?: string; repoUrl?: string; ref?: string } | null {
  if (!outputs) return null;
  const slot = outputs[nodeName];
  if (!slot || typeof slot !== "object") return null;
  const portMap = slot as Record<string, unknown>;
  const port = portMap["0"];
  if (!Array.isArray(port) || port.length === 0) return null;
  const item = port[0] as Record<string, unknown>;
  // Heuristic: trigger items carry source/agentId/repoUrl/ref.
  if (!item.source && !item.repoUrl) return null;
  return {
    source: typeof item.source === "string" ? item.source : undefined,
    repoUrl: typeof item.repoUrl === "string" ? item.repoUrl : undefined,
    ref: typeof item.ref === "string" ? item.ref : undefined,
  };
}
