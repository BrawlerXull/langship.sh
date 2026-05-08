"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Plus, RefreshCw, Trash2, Activity } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { api, type FlowSummary } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function DashboardPage() {
  const [flows, setFlows] = useState<FlowSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [health, setHealth] = useState<"ok" | "down" | "checking">("checking");

  async function load() {
    try {
      const list = await api.listFlows();
      list.sort((a, b) => (b.updatedAt || "").localeCompare(a.updatedAt || ""));
      setFlows(list);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load");
    }
  }

  useEffect(() => {
    load();
    api
      .health()
      .then(() => setHealth("ok"))
      .catch(() => setHealth("down"));
  }, []);

  async function onDelete(id: string) {
    if (!confirm("Delete this pipeline?")) return;
    try {
      await api.deleteFlow(id);
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <div className="space-y-8 p-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Pipelines</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Durable, n8n-compatible pipelines. Build on the canvas or paste exported
            JSON to import.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <span
              className={
                health === "ok"
                  ? "h-2 w-2 rounded-full bg-emerald-500"
                  : health === "down"
                    ? "h-2 w-2 rounded-full bg-rose-500"
                    : "h-2 w-2 rounded-full bg-amber-500"
              }
            />
            <span>API {health}</span>
          </div>
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw />
            Refresh
          </Button>
          <Button size="sm" asChild>
            <Link href="/flows/new">
              <Plus />
              New pipeline
            </Link>
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {flows === null ? (
        <SkeletonGrid />
      ) : flows.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {flows.map((f) => (
            <Card key={f.id} className="group transition-shadow hover:shadow-md">
              <CardHeader>
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <CardTitle className="truncate">
                      {f.name || "Untitled pipeline"}
                    </CardTitle>
                    <CardDescription className="mt-1 truncate font-mono text-[11px]">
                      {f.id}
                    </CardDescription>
                  </div>
                  <Badge variant="secondary">{f.status || "draft"}</Badge>
                </div>
              </CardHeader>
              <CardContent className="flex items-center justify-between text-sm text-muted-foreground">
                <div className="flex items-center gap-3">
                  <span>
                    <span className="font-medium text-foreground">{f.nodeCount}</span>{" "}
                    nodes
                  </span>
                  <span>·</span>
                  <span>{formatDate(f.updatedAt)}</span>
                </div>
                <div className="flex items-center gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                  <Button size="sm" variant="ghost" asChild>
                    <Link href={`/flows/view/?id=${encodeURIComponent(f.id)}`}>
                      <Activity />
                      Open
                    </Link>
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => onDelete(f.id)}
                    aria-label="Delete pipeline"
                  >
                    <Trash2 />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

function SkeletonGrid() {
  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      {Array.from({ length: 3 }).map((_, i) => (
        <Card key={i}>
          <CardHeader>
            <div className="h-4 w-1/2 animate-pulse rounded bg-muted" />
            <div className="mt-2 h-3 w-1/3 animate-pulse rounded bg-muted" />
          </CardHeader>
          <CardContent>
            <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
          </CardContent>
        </Card>
      ))}
    </div>
  );
}

function EmptyState() {
  return (
    <Card className="border-dashed">
      <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
        <div className="rounded-full bg-muted p-3">
          <Plus className="h-5 w-5 text-muted-foreground" />
        </div>
        <div>
          <p className="text-sm font-medium">No pipelines yet</p>
          <p className="text-sm text-muted-foreground">
            Build one on the canvas, or import an n8n workflow JSON.
          </p>
        </div>
        <Button size="sm" asChild>
          <Link href="/flows/new">Create your first pipeline</Link>
        </Button>
      </CardContent>
    </Card>
  );
}
