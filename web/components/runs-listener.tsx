"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Play, X } from "lucide-react";
import { api } from "@/lib/api";

interface RunCreated {
  type: string;
  executionId: string;
  pipelineId?: string;
  pipelineName?: string;
  agentId?: string;
  source?: string;
  startedAt?: string;
}

type Toast = RunCreated & { receivedAt: number };

// RunsListener mounts once at the app root, opens an SSE stream to
// /api/runs/stream, and renders a stack of "new run" toasts in the
// bottom-right corner. Each toast links to the live run view.
//
// Toasts auto-dismiss after 12s. Clicking the stream's link navigates to
// the live execution page (which has its own per-execution SSE feed).
export function RunsListener() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  useEffect(() => {
    let es: EventSource | null = null;
    let backoff = 1000; // ms — grows on consecutive errors, capped at 30s

    function connect() {
      es = new EventSource(api.runsStreamURL());
      es.onopen = () => {
        backoff = 1000;
      };
      es.onerror = () => {
        es?.close();
        es = null;
        // EventSource doesn't reconnect cleanly when nginx / cloudflared
        // drops the connection — fall back to manual reconnect with
        // bounded backoff so we keep getting events after a server restart.
        const wait = Math.min(backoff, 30_000);
        backoff = Math.min(backoff * 2, 30_000);
        setTimeout(connect, wait);
      };
      es.onmessage = (msg) => {
        try {
          const ev: RunCreated = JSON.parse(msg.data);
          if (ev.type !== "run_created" || !ev.executionId) return;
          setToasts((prev) => {
            const next = [...prev, { ...ev, receivedAt: Date.now() }];
            // Cap visible stack to 4
            return next.slice(-4);
          });
        } catch {
          /* ignore */
        }
      };
    }
    connect();
    return () => {
      es?.close();
    };
  }, []);

  // Auto-dismiss
  useEffect(() => {
    if (toasts.length === 0) return;
    const t = setInterval(() => {
      const cutoff = Date.now() - 12_000;
      setToasts((prev) => prev.filter((x) => x.receivedAt > cutoff));
    }, 1000);
    return () => clearInterval(t);
  }, [toasts.length]);

  function dismiss(id: string) {
    setToasts((prev) => prev.filter((t) => t.executionId !== id));
  }

  if (toasts.length === 0) return null;

  return (
    <div className="pointer-events-none fixed bottom-6 right-6 z-40 flex flex-col gap-2">
      {toasts.map((t) => (
        <ToastCard key={t.executionId} toast={t} onDismiss={dismiss} />
      ))}
    </div>
  );
}

function ToastCard({
  toast,
  onDismiss,
}: {
  toast: Toast;
  onDismiss: (id: string) => void;
}) {
  const sourceLabel =
    toast.source === "github_push"
      ? "GitHub push"
      : toast.source === "manual"
        ? "Manual trigger"
        : (toast.source ?? "New run");

  return (
    <div className="pointer-events-auto flex w-80 items-start gap-3 rounded-lg border bg-background p-3 shadow-xl">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-emerald-500/15 text-emerald-600 dark:text-emerald-400">
        <Play className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{sourceLabel}</div>
        <div className="truncate text-xs text-muted-foreground">
          {toast.pipelineName || toast.pipelineId || "pipeline"}
        </div>
        <Link
          href={`/executions/view/?id=${encodeURIComponent(toast.executionId)}`}
          className="mt-1 inline-block text-xs font-medium text-primary hover:underline"
        >
          Open live view →
        </Link>
      </div>
      <button
        type="button"
        onClick={() => onDismiss(toast.executionId)}
        className="shrink-0 rounded-md p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
        aria-label="Dismiss"
      >
        <X className="size-3.5" />
      </button>
    </div>
  );
}
