"use client";

import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Check, ChevronDown, ChevronRight, Pause, RefreshCw, Send, X } from "lucide-react";
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
  const [approvalReason, setApprovalReason] = useState("");

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

  // Poll the orchestrator REST endpoint independently of the SSE feed so
  // we pick up `pending_approval` as soon as the Approval node parks the
  // workflow. SSE only carries the engine's lifecycle events; pending-
  // approval state is set by the executor as Restate KV and surfaced via
  // GetPendingApproval. Don't poll once we've reached terminal state.
  // 4s interval matches Restate's recommended minimum to avoid the
  // shared-handler racing the workflow's own goroutine.
  useEffect(() => {
    if (!id) return;
    const isTerm = (s: string | undefined) =>
      ["success", "completed", "failed", "error", "partial_error"].includes(
        (s || "").toLowerCase()
      );
    if (isTerm(status?.status)) return;
    const t = setInterval(() => {
      api.getExecution(id).then(setStatus).catch(() => {});
    }, 4000);
    return () => clearInterval(t);
  }, [id, status?.status]);

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

  // approveOrReject is the one-click path: reads the awakeable id from
  // pending_approval (no manual copy) and resolves with the standard
  // {approved: bool, reason} body the ApprovalExecutor unwraps.
  async function approveOrReject(approved: boolean, reason?: string) {
    const pending = (status as { pending_approval?: { awakeable_id?: string } } | null)
      ?.pending_approval;
    if (!pending?.awakeable_id) {
      setError("no pending approval on this run");
      return;
    }
    setResuming(true);
    setError(null);
    try {
      await api.resumeExecution(id, {
        awakeable_id: pending.awakeable_id,
        data: { approved, reason },
      });
      const s = await api.getExecution(id).catch(() => null);
      if (s) setStatus(s);
    } catch (e) {
      setError(e instanceof Error ? e.message : "resume failed");
    } finally {
      setResuming(false);
    }
  }

  const pendingApproval = useMemo(() => {
    const pa = (status as { pending_approval?: { node?: string; awakeable_id?: string; context?: Record<string, unknown> } } | null)
      ?.pending_approval;
    return pa && pa.awakeable_id ? pa : null;
  }, [status]);

  const overallStatus = useMemo(
    () =>
      pendingApproval
        ? "paused"
        : (status?.status as string) || run?.status || "running",
    [status, run, pendingApproval]
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

      {/* Pending approval overlay — surfaces the awakeable, exposes
          one-click Approve / Reject so the user never has to copy IDs. */}
      {pendingApproval && (
        <Card className="fixed bottom-6 right-6 z-30 w-96 border-amber-500/50 shadow-xl">
          <CardHeader className="space-y-1 border-b border-amber-500/20 bg-amber-500/5">
            <div className="flex items-center justify-between gap-2">
              <CardTitle className="flex items-center gap-2 text-amber-700 dark:text-amber-400">
                <Pause className="size-4" />
                Approval needed
              </CardTitle>
              <Badge variant="warning">{pendingApproval.node}</Badge>
            </div>
            <CardDescription>
              {pendingReason(pendingApproval) ??
                "This run is waiting for a human decision."}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 pt-3">
            <div className="space-y-1.5">
              <Label htmlFor="approval-reason">
                Reason (optional, recorded in audit)
              </Label>
              <Textarea
                id="approval-reason"
                rows={2}
                value={approvalReason}
                onChange={(e) => setApprovalReason(e.target.value)}
                spellCheck
                placeholder="LGTM — image scan clean, deploy to dev"
                className="text-xs"
              />
            </div>
            <div className="grid grid-cols-2 gap-2">
              <Button
                onClick={() => approveOrReject(true, approvalReason)}
                disabled={resuming}
                className="bg-emerald-600 hover:bg-emerald-700 text-white"
              >
                <Check className="size-4" />
                {resuming ? "Sending…" : "Approve"}
              </Button>
              <Button
                onClick={() => approveOrReject(false, approvalReason)}
                disabled={resuming}
                variant="destructive"
              >
                <X className="size-4" />
                Reject
              </Button>
            </div>
            <details className="rounded-md border bg-muted/20 p-2">
              <summary className="cursor-pointer text-[10px] uppercase tracking-wider text-muted-foreground">
                Manual resolve (raw awakeable)
              </summary>
              <div className="mt-2 space-y-2">
                <div className="font-mono text-[10px] break-all text-muted-foreground">
                  {pendingApproval.awakeable_id}
                </div>
                <Textarea
                  rows={3}
                  value={data}
                  onChange={(e) => setData(e.target.value)}
                  spellCheck={false}
                  className="text-[10px] font-mono"
                  placeholder='{"approved": true, "reason": "…"}'
                />
                <Button
                  size="sm"
                  variant="outline"
                  className="w-full"
                  disabled={resuming}
                  onClick={() => {
                    setAwakeable(pendingApproval.awakeable_id ?? "");
                    onResume();
                  }}
                >
                  <Send className="size-3.5" />
                  Send raw
                </Button>
              </div>
            </details>
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
  const sastSummary = extractScanSummary(node.name, statusOutputs, "__sast");
  const imageScanSummary = extractScanSummary(node.name, statusOutputs, "__imageScan");
  const pushSummary = extractPushSummary(node.name, statusOutputs);

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

      {sastSummary && (
        <ScanResults summary={sastSummary} kind="sast" />
      )}
      {imageScanSummary && (
        <ScanResults summary={imageScanSummary} kind="imageScan" />
      )}

      {pushSummary && (
        <div className="border-t px-4 py-3 font-mono text-[11px] leading-6">
          <div>
            <span className="text-muted-foreground">src: </span>
            <span className="break-all">{pushSummary.src}</span>
          </div>
          {pushSummary.copies.map((c, i) => (
            <div key={i}>
              <span className="text-muted-foreground">
                → {c.name || c.registry}:{" "}
              </span>
              <span className="break-all">{c.imageRef}</span>
              {c.error ? (
                <span className="ml-1 text-rose-500">— {c.error}</span>
              ) : c.digest ? (
                <span className="ml-1 text-emerald-600 dark:text-emerald-500">
                  {" "}
                  ✓{" "}
                  <span className="text-muted-foreground">
                    {c.digest.slice(0, 19)}…
                  </span>
                </span>
              ) : null}
            </div>
          ))}
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

// --- scan / push helpers -------------------------------------------------

type ScanFinding = {
  tool?: string;
  severity?: string;
  ruleId?: string;
  file?: string;
  line?: number;
  message?: string;
};

type ScanSummary = {
  tool?: string;
  imageRef?: string;
  scanRef?: string;
  qualityGate?: string;
  dashboardUrl?: string;
  threshold?: string;
  counts: Record<string, number>;
  findingCount: number;
  findings: ScanFinding[];
};

function extractScanSummary(
  nodeName: string,
  outputs: Record<string, unknown> | undefined,
  key: "__sast" | "__imageScan"
): ScanSummary | null {
  if (!outputs) return null;
  const port = ((outputs[nodeName] as Record<string, unknown>) || {})["0"];
  if (!Array.isArray(port) || port.length === 0) return null;
  const item = port[0] as Record<string, unknown>;
  const blob = item[key] as Record<string, unknown> | undefined;
  if (!blob) return null;

  const counts: Record<string, number> = {};
  if (blob.counts && typeof blob.counts === "object") {
    for (const [k, v] of Object.entries(blob.counts as Record<string, unknown>)) {
      if (typeof v === "number") counts[k.toUpperCase()] = v;
    }
  }
  const findings = Array.isArray(blob.findings) ? (blob.findings as ScanFinding[]) : [];
  return {
    tool: typeof blob.tool === "string" ? blob.tool : undefined,
    imageRef: typeof blob.imageRef === "string" ? blob.imageRef : undefined,
    scanRef: typeof blob.scanRef === "string" ? blob.scanRef : undefined,
    qualityGate: typeof blob.qualityGate === "string" ? blob.qualityGate : undefined,
    dashboardUrl: typeof blob.dashboardUrl === "string" ? blob.dashboardUrl : undefined,
    threshold:
      typeof blob.severityThreshold === "string"
        ? (blob.severityThreshold as string)
        : undefined,
    counts,
    findingCount:
      typeof blob.finding_count === "number"
        ? (blob.finding_count as number)
        : findings.length,
    findings,
  };
}

type PushSummary = {
  src: string;
  copies: { name?: string; registry?: string; imageRef: string; digest?: string; error?: string }[];
};

function extractPushSummary(
  nodeName: string,
  outputs?: Record<string, unknown>
): PushSummary | null {
  if (!outputs) return null;
  const port = ((outputs[nodeName] as Record<string, unknown>) || {})["0"];
  if (!Array.isArray(port) || port.length === 0) return null;
  const item = port[0] as Record<string, unknown>;
  const blob = item.__push as Record<string, unknown> | undefined;
  if (!blob) return null;
  const copies = Array.isArray(blob.copies)
    ? (blob.copies as Record<string, unknown>[]).map((c) => ({
        name: typeof c.name === "string" ? c.name : undefined,
        registry: typeof c.registry === "string" ? c.registry : undefined,
        imageRef: typeof c.imageRef === "string" ? c.imageRef : "",
        digest: typeof c.digest === "string" ? c.digest : undefined,
        error: typeof c.error === "string" && c.error ? c.error : undefined,
      }))
    : [];
  // Single-target legacy shape — promote dst to a one-entry copies array.
  if (copies.length === 0 && typeof blob.dst === "string") {
    copies.push({
      name: undefined,
      registry: undefined,
      imageRef: blob.dst as string,
      digest: typeof blob.digest === "string" ? (blob.digest as string) : undefined,
      error: undefined,
    });
  }
  return {
    src: typeof blob.src === "string" ? (blob.src as string) : "",
    copies,
  };
}

const SEVERITY_ORDER = ["CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"] as const;

function severityClass(sev: string): string {
  switch (sev.toUpperCase()) {
    case "CRITICAL":
      return "bg-rose-600/20 text-rose-700 dark:text-rose-400 border-rose-600/40";
    case "HIGH":
      return "bg-orange-500/20 text-orange-700 dark:text-orange-400 border-orange-500/40";
    case "MEDIUM":
      return "bg-amber-500/20 text-amber-700 dark:text-amber-400 border-amber-500/40";
    case "LOW":
      return "bg-sky-500/20 text-sky-700 dark:text-sky-400 border-sky-500/40";
    default:
      return "bg-muted text-muted-foreground border-border";
  }
}

function ScanResults({
  summary,
  kind,
}: {
  summary: ScanSummary;
  kind: "sast" | "imageScan";
}) {
  const [open, setOpen] = useState(false);

  const total = summary.findingCount;
  const showFindings = summary.findings && summary.findings.length > 0;
  const isSonar = summary.tool === "sonar";
  const headerLabel = kind === "sast" ? "SAST" : "Image scan";

  return (
    <div className="border-t">
      <div className="flex items-center justify-between gap-3 px-4 py-3 text-xs">
        <div className="flex flex-wrap items-center gap-2">
          <span className="font-medium uppercase tracking-wider text-muted-foreground">
            {headerLabel}
          </span>
          {summary.tool && (
            <span className="rounded-md border bg-muted/40 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wider">
              {summary.tool}
            </span>
          )}
          {summary.threshold && (
            <span className="text-[10px] text-muted-foreground">
              threshold {summary.threshold}
            </span>
          )}
          {/* Severity counts pills */}
          {SEVERITY_ORDER.map((sev) => {
            const n = summary.counts[sev] || 0;
            if (n === 0) return null;
            return (
              <span
                key={sev}
                className={
                  "rounded-md border px-2 py-0.5 text-[10px] font-medium " +
                  severityClass(sev)
                }
              >
                {sev.toLowerCase()} {n}
              </span>
            );
          })}
          {total === 0 && !isSonar && (
            <span className="rounded-md border border-emerald-500/40 bg-emerald-500/15 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">
              clean
            </span>
          )}
          {isSonar && summary.qualityGate && (
            <span
              className={
                "rounded-md border px-2 py-0.5 text-[10px] font-medium " +
                (summary.qualityGate === "OK"
                  ? "bg-emerald-500/15 text-emerald-700 border-emerald-500/40 dark:text-emerald-400"
                  : summary.qualityGate === "WARN"
                    ? "bg-amber-500/20 text-amber-700 border-amber-500/40 dark:text-amber-400"
                    : "bg-rose-500/20 text-rose-700 border-rose-500/40 dark:text-rose-400")
              }
            >
              gate {summary.qualityGate}
            </span>
          )}
        </div>
        {showFindings && (
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            className="flex items-center gap-1 rounded-md border bg-background px-2 py-0.5 text-[10px] hover:bg-accent"
          >
            {open ? (
              <ChevronDown className="size-3" />
            ) : (
              <ChevronRight className="size-3" />
            )}
            {total} {total === 1 ? "finding" : "findings"}
          </button>
        )}
        {summary.dashboardUrl && (
          <a
            href={summary.dashboardUrl}
            target="_blank"
            rel="noreferrer"
            className="text-[10px] text-primary underline-offset-4 hover:underline"
          >
            open dashboard ↗
          </a>
        )}
      </div>

      {open && showFindings && (
        <div className="border-t bg-muted/10 px-4 py-2">
          <div className="max-h-72 overflow-auto">
            <table className="w-full text-left text-[11px]">
              <thead className="text-muted-foreground">
                <tr>
                  <th className="py-1 pr-2">severity</th>
                  <th className="py-1 pr-2">rule</th>
                  <th className="py-1 pr-2">where</th>
                  <th className="py-1">message</th>
                </tr>
              </thead>
              <tbody>
                {summary.findings.slice(0, 100).map((f, i) => (
                  <tr key={i} className="border-t border-border/50">
                    <td className="py-1 pr-2 align-top">
                      <span
                        className={
                          "rounded-md border px-1.5 py-0.5 text-[10px] font-medium " +
                          severityClass(f.severity ?? "UNKNOWN")
                        }
                      >
                        {(f.severity ?? "?").toLowerCase()}
                      </span>
                    </td>
                    <td className="py-1 pr-2 align-top font-mono text-[10px]">
                      {f.ruleId ?? ""}
                    </td>
                    <td className="py-1 pr-2 align-top font-mono text-[10px] text-muted-foreground">
                      {f.file ? `${f.file}${f.line ? ":" + f.line : ""}` : ""}
                    </td>
                    <td className="py-1 align-top">{f.message ?? ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {summary.findings.length > 100 && (
              <div className="mt-1 text-[10px] text-muted-foreground">
                + {summary.findings.length - 100} more (truncated)
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// pendingReason returns a human-readable label for a pending approval. The
// ApprovalExecutor stores its `reason` parameter under `context.reason` so
// it round-trips through the workflow journal — pull it back out here.
function pendingReason(pa: {
  context?: Record<string, unknown>;
}): string | undefined {
  const r = pa.context?.reason;
  return typeof r === "string" && r.trim() ? r : undefined;
}
