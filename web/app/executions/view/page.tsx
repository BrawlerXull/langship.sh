"use client";

import { Suspense, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ArrowLeft, RefreshCw, Send } from "lucide-react";
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
import type { PipelineDefinition } from "@/lib/pipeline-graph";

type NodeStatus = "pending" | "running" | "success" | "failed" | "paused";
type NodeStatuses = Record<string, NodeStatus>;

interface ExecutionEvent {
  type: string;
  node?: string;
  node_type?: string;
  status?: string;
  content?: string; // for node_log events
  outputs?: Record<string, unknown>;
  error?: string;
  duration_ms?: number;
}

type NodeLogLine = { ts: number; line: string };
type NodeLogs = Record<string, NodeLogLine[]>;

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
  const [events, setEvents] = useState<ExecutionEvent[]>([]);
  const [nodeLogs, setNodeLogs] = useState<NodeLogs>({});
  const [activeLogNode, setActiveLogNode] = useState<string | null>(null);
  const [streamConnected, setStreamConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<"canvas" | "json">("canvas");

  // Resume form
  const [awakeable, setAwakeable] = useState("");
  const [data, setData] = useState(`{"approved": true}`);
  const [resuming, setResuming] = useState(false);

  const eventSourceRef = useRef<EventSource | null>(null);

  // Initial load: status + run record + pipeline def.
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
            // Pipeline may have been deleted; render JSON view only.
          }
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "load failed");
      }
    })();
  }, [id]);

  // SSE subscription. On open, mark all known nodes as pending. Each
  // node_started/completed/error event flips that node's status.
  useEffect(() => {
    if (!id) return;
    const url = api.executionStreamURL(id);
    const es = new EventSource(url);
    eventSourceRef.current = es;
    es.onopen = () => setStreamConnected(true);
    es.onerror = () => {
      setStreamConnected(false);
      // EventSource auto-reconnects on transient errors; only close on
      // permanent ones. We log but don't bail.
    };
    es.onmessage = (msg) => {
      try {
        const ev: ExecutionEvent = JSON.parse(msg.data);

        // Log lines are high-frequency — keep them out of the generic
        // events array (used for the JSON debug view) and route into
        // their own state instead.
        if (ev.type === "node_log" && ev.node && typeof ev.content === "string") {
          const node = ev.node;
          setNodeLogs((prev) => {
            const cur = prev[node] ?? [];
            const next = cur.length >= 2000 ? cur.slice(-1999) : cur;
            return {
              ...prev,
              [node]: [...next, { ts: Date.now(), line: ev.content! }],
            };
          });
          setActiveLogNode((prev) => prev ?? node);
          return;
        }

        setEvents((prev) => [...prev.slice(-99), ev]);
        if (ev.node) {
          setNodeStatuses((prev) => {
            const next: NodeStatus =
              ev.type === "node_started"
                ? "running"
                : ev.type === "node_completed"
                  ? "success"
                  : ev.type === "node_error"
                    ? "failed"
                    : (prev[ev.node!] ?? "pending");
            return { ...prev, [ev.node!]: next };
          });
          if (ev.type === "node_started") {
            setActiveLogNode(ev.node);
          }
        }
        if (ev.type === "done") {
          api.getExecution(id).then(setStatus).catch(() => {});
          es.close();
          setStreamConnected(false);
        }
      } catch {
        // Ignore malformed messages.
      }
    };

    return () => {
      es.close();
      eventSourceRef.current = null;
      setStreamConnected(false);
    };
  }, [id]);

  // Fallback polling for terminal status — useful if SSE failed to connect.
  useEffect(() => {
    if (!id || streamConnected) return;
    const t = setInterval(() => {
      api.getExecution(id).then(setStatus).catch(() => {});
    }, 3000);
    return () => clearInterval(t);
  }, [id, streamConnected]);

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

  const overallStatus = useMemo(() => {
    return (status?.status as string) || run?.status || "running";
  }, [status, run]);

  if (!id) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        Missing <code>id</code> query param.
      </div>
    );
  }

  return (
    <div className="flex h-screen flex-col">
      {/* Toolbar */}
      <div className="flex h-12 shrink-0 items-center gap-2 border-b bg-background/80 px-3 backdrop-blur">
        <Button variant="ghost" size="icon" asChild aria-label="Back">
          <Link href="/">
            <ArrowLeft className="size-4" />
          </Link>
        </Button>
        <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
          Execution
        </div>
        <span className="font-mono text-xs">{id}</span>
        <Badge variant={statusVariant(overallStatus)}>{overallStatus}</Badge>
        {streamConnected ? (
          <Badge variant="success" className="text-[10px]">
            ● live
          </Badge>
        ) : (
          <Badge variant="outline" className="text-[10px]">
            polling
          </Badge>
        )}
        <div className="flex-1" />
        <div className="inline-flex rounded-md border bg-muted/30 p-0.5">
          <button
            type="button"
            onClick={() => setTab("canvas")}
            className={
              "rounded-sm px-3 py-1 text-xs " +
              (tab === "canvas"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground")
            }
          >
            Canvas
          </button>
          <button
            type="button"
            onClick={() => setTab("json")}
            className={
              "rounded-sm px-3 py-1 text-xs " +
              (tab === "json"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground")
            }
          >
            JSON
          </button>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => api.getExecution(id).then(setStatus).catch(() => {})}
        >
          <RefreshCw />
          Refresh
        </Button>
      </div>

      {error && (
        <div className="border-b border-destructive/40 bg-destructive/5 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="relative min-h-0 flex-1">
        {tab === "canvas" ? (
          <div className="flex h-full flex-col">
            <div className="relative min-h-0 flex-1">
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
                    : "No pipeline definition available for this run."}
                </div>
              )}
            </div>
            <LogPanel
              nodeLogs={nodeLogs}
              activeNode={activeLogNode}
              onActiveNodeChange={setActiveLogNode}
              nodeStatuses={nodeStatuses}
              streaming={streamConnected}
            />
          </div>
        ) : (
          <div className="grid h-full grid-cols-1 gap-4 overflow-auto p-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Status</CardTitle>
                <CardDescription>Latest snapshot from the orchestrator</CardDescription>
              </CardHeader>
              <CardContent>
                <pre className="max-h-[60vh] overflow-auto rounded-md border bg-muted/30 p-3 text-xs">
                  {status ? JSON.stringify(status, null, 2) : "Loading…"}
                </pre>
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle>Event log</CardTitle>
                <CardDescription>Streamed via SSE</CardDescription>
              </CardHeader>
              <CardContent>
                <pre className="max-h-[60vh] overflow-auto rounded-md border bg-muted/30 p-3 text-[11px]">
                  {events.length === 0
                    ? "(no events yet)"
                    : events
                        .map((e) => JSON.stringify(e))
                        .join("\n")}
                </pre>
              </CardContent>
            </Card>
          </div>
        )}

        {/* Resume panel — overlaid only when paused */}
        {(overallStatus === "paused" || overallStatus === "waiting") && (
          <Card className="absolute right-4 top-4 z-20 w-80 shadow-xl">
            <CardHeader>
              <CardTitle>Resume</CardTitle>
              <CardDescription>
                Resolve the Restate awakeable to continue.
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
      </div>
    </div>
  );
}

// LogPanel renders the per-node live log output below the canvas. Tabs at
// the top let the user pick which node's stream they're watching; the
// active tab follows the most-recently-started node by default.
function LogPanel(props: {
  nodeLogs: NodeLogs;
  activeNode: string | null;
  onActiveNodeChange: (node: string | null) => void;
  nodeStatuses: NodeStatuses;
  streaming: boolean;
}) {
  const { nodeLogs, activeNode, onActiveNodeChange, nodeStatuses, streaming } = props;
  const nodes = Object.keys(nodeLogs);
  const lines = activeNode ? (nodeLogs[activeNode] ?? []) : [];
  const scrollRef = useRef<HTMLDivElement>(null);

  // Auto-scroll to bottom on new lines unless the user has scrolled up.
  const stickRef = useRef(true);
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    if (stickRef.current) {
      el.scrollTop = el.scrollHeight;
    }
  }, [lines.length, activeNode]);

  function onScroll() {
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
    stickRef.current = atBottom;
  }

  return (
    <div className="flex h-64 shrink-0 flex-col border-t bg-background">
      <div className="flex items-center gap-1 overflow-x-auto border-b px-2 py-1">
        <span className="px-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          Logs
        </span>
        {nodes.length === 0 ? (
          <span className="px-2 text-xs text-muted-foreground">
            {streaming ? "waiting for output…" : "no logs yet"}
          </span>
        ) : (
          nodes.map((n) => {
            const status = nodeStatuses[n];
            const isActive = activeNode === n;
            return (
              <button
                key={n}
                onClick={() => onActiveNodeChange(n)}
                className={
                  "flex items-center gap-1.5 rounded-md px-2 py-1 text-xs transition-colors " +
                  (isActive
                    ? "bg-muted text-foreground"
                    : "text-muted-foreground hover:bg-muted/60")
                }
              >
                <span
                  className={
                    "size-1.5 rounded-full " +
                    (status === "running"
                      ? "bg-sky-500 animate-pulse"
                      : status === "success"
                        ? "bg-emerald-500"
                        : status === "failed"
                          ? "bg-rose-500"
                          : "bg-muted-foreground/60")
                  }
                />
                <span className="truncate max-w-[160px]">{n}</span>
                <span className="text-[10px] text-muted-foreground">
                  {nodeLogs[n].length}
                </span>
              </button>
            );
          })
        )}
        <div className="flex-1" />
        {streaming && (
          <span className="px-2 text-[10px] text-emerald-600 dark:text-emerald-500">
            ● live
          </span>
        )}
      </div>
      <div
        ref={scrollRef}
        onScroll={onScroll}
        className="flex-1 overflow-auto bg-zinc-950 px-3 py-2 font-mono text-[11px] leading-[1.4] text-zinc-200"
      >
        {lines.length === 0 ? (
          <div className="text-zinc-500">
            {activeNode
              ? "waiting for output…"
              : "select a node tab above"}
          </div>
        ) : (
          lines.map((l, i) => (
            <div key={i} className="whitespace-pre-wrap break-all">
              {l.line}
            </div>
          ))
        )}
      </div>
    </div>
  );
}
