"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  Check,
  Pause,
  RefreshCw,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Textarea } from "@/components/ui/textarea";
import { api, type ExecutionStatus, type Run } from "@/lib/api";
import { formatDate } from "@/lib/utils";

interface PendingApproval {
  awakeable_id: string;
  node?: string;
  context?: { reason?: string; node?: string; [k: string]: unknown };
}

interface PendingItem {
  run: Run;
  approval: PendingApproval;
}

// /approvals lists every run that is currently parked on a HITL Approval
// node. The orchestrator stores `pending_approval` as Restate KV and
// surfaces it via GetExecution; we fan out per "running" run, keep only
// the ones that have a pending_approval set, and let the user resolve
// them in one click.
export default function ApprovalsInboxPage() {
  const [items, setItems] = useState<PendingItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const loadRef = useRef(0);

  const load = useCallback(async () => {
    const tag = ++loadRef.current;
    try {
      const runs = await api.listRuns({ limit: 50 });
      const candidates = runs.filter((r) => /running|pending|paused|waiting/i.test(r.status));
      const checked = await Promise.all(
        candidates.map(async (r) => {
          try {
            const s = (await api.getExecution(r.id)) as ExecutionStatus & {
              pending_approval?: PendingApproval;
            };
            const pa = s.pending_approval;
            if (pa && pa.awakeable_id) {
              return { run: r, approval: pa } as PendingItem;
            }
          } catch {
            /* ignore — run may have advanced */
          }
          return null;
        })
      );
      // Only commit if a newer load hasn't started.
      if (tag !== loadRef.current) return;
      setItems(checked.filter(Boolean) as PendingItem[]);
      setError(null);
    } catch (e) {
      if (tag !== loadRef.current) return;
      setError(e instanceof Error ? e.message : "load failed");
    }
  }, []);

  useEffect(() => {
    load();
    // 6s — same reasoning as executions/view: avoid racing the workflow
    // SDK's shared-handler with parallel reads.
    const t = setInterval(load, 6000);
    // Also reload when ANY new run is created (covers the case where a
    // freshly-triggered run instantly parks on an Approval node).
    const es = new EventSource(api.runsStreamURL());
    es.onmessage = () => load();
    es.onerror = () => {};
    return () => {
      clearInterval(t);
      es.close();
    };
  }, [load]);

  async function decide(it: PendingItem, approved: boolean, reason: string) {
    setBusyId(it.run.id);
    try {
      await api.resumeExecution(it.run.id, {
        awakeable_id: it.approval.awakeable_id,
        data: { approved, reason },
      });
      await load();
    } catch (e) {
      setError(e instanceof Error ? e.message : "resume failed");
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Approvals</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Runs paused on a HITL Approval node, awaiting a decision.
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={load}>
          <RefreshCw />
          Refresh
        </Button>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {items === null ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : items.length === 0 ? (
        <Card className="border-dashed">
          <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
            <Pause className="size-5 text-muted-foreground" />
            <div className="text-sm font-medium">No approvals waiting</div>
            <p className="max-w-sm text-sm text-muted-foreground">
              When a pipeline hits a Wait-for-approval node it&rsquo;ll show up here
              for a one-click decision.
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-3">
          {items.map((it) => (
            <ApprovalCard
              key={it.run.id}
              item={it}
              busy={busyId === it.run.id}
              onDecide={decide}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function ApprovalCard({
  item,
  busy,
  onDecide,
}: {
  item: PendingItem;
  busy: boolean;
  onDecide: (it: PendingItem, approved: boolean, reason: string) => void;
}) {
  const [reason, setReason] = useState("");
  const reasonHint =
    typeof item.approval.context?.reason === "string"
      ? (item.approval.context.reason as string)
      : undefined;
  const nodeName =
    item.approval.node ||
    (typeof item.approval.context?.node === "string"
      ? (item.approval.context.node as string)
      : "Approval");

  return (
    <Card className="border-amber-500/40">
      <CardHeader className="border-b border-amber-500/20 bg-amber-500/5 py-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0">
            <CardTitle className="flex items-center gap-2 text-base">
              <Pause className="size-4 text-amber-600 dark:text-amber-400" />
              {item.run.pipelineName || "Pipeline"}
              <Badge variant="warning">{nodeName}</Badge>
            </CardTitle>
            <CardDescription className="mt-1 truncate font-mono text-[10px]">
              {item.run.id}
            </CardDescription>
          </div>
          <Button size="sm" variant="ghost" asChild>
            <Link href={`/executions/view/?id=${encodeURIComponent(item.run.id)}`}>
              Open run <ArrowRight className="size-3.5" />
            </Link>
          </Button>
        </div>
        {reasonHint && (
          <p className="mt-1 text-sm text-muted-foreground">{reasonHint}</p>
        )}
        <div className="mt-1 text-xs text-muted-foreground">
          paused since {formatDate(item.run.startedAt)}
        </div>
      </CardHeader>
      <CardContent className="space-y-3 pt-3">
        <div className="space-y-1.5">
          <Textarea
            rows={2}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            spellCheck
            placeholder="Decision note (optional, recorded in audit)"
            className="text-xs"
          />
        </div>
        <div className="flex gap-2">
          <Button
            onClick={() => onDecide(item, true, reason)}
            disabled={busy}
            className="flex-1 bg-emerald-600 hover:bg-emerald-700 text-white"
          >
            <Check className="size-4" />
            {busy ? "Sending…" : "Approve"}
          </Button>
          <Button
            onClick={() => onDecide(item, false, reason)}
            disabled={busy}
            variant="destructive"
            className="flex-1"
          >
            <X className="size-4" />
            Reject
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
