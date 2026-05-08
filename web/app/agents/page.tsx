"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  Bot,
  ExternalLink,
  KeyRound,
  Plus,
  RefreshCw,
  Trash2,
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
import { api, type Agent } from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function load() {
    try {
      const list = await api.listAgents();
      list.sort((a, b) => (b.updatedAt || "").localeCompare(a.updatedAt || ""));
      setAgents(list);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "failed to load");
    }
  }

  useEffect(() => {
    load();
  }, []);

  async function onDelete(id: string) {
    if (!confirm("Remove this agent?")) return;
    try {
      await api.deleteAgent(id);
      await load();
    } catch (e) {
      alert(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <div className="space-y-8 p-6">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">Agents</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Agent repositories Langship watches and deploys. Add a git URL — we
            track the ref and trigger pipelines on push.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={load}>
            <RefreshCw />
            Refresh
          </Button>
          <Button size="sm" asChild>
            <Link href="/agents/new">
              <Plus />
              Add agent
            </Link>
          </Button>
        </div>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {agents === null ? (
        <SkeletonGrid />
      ) : agents.length === 0 ? (
        <EmptyState />
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {agents.map((a) => (
            <Link
              key={a.id}
              href={`/agents/view/?id=${encodeURIComponent(a.id)}`}
              className="group block"
            >
              <Card className="h-full transition-shadow hover:shadow-md">
                <CardHeader>
                  <div className="flex items-start justify-between gap-2">
                    <div className="min-w-0 flex items-center gap-2">
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                        <Bot className="size-4" />
                      </span>
                      <div className="min-w-0">
                        <CardTitle className="truncate">{a.name}</CardTitle>
                        <CardDescription className="mt-0.5 truncate text-[11px]">
                          {a.ref || "main"}
                        </CardDescription>
                      </div>
                    </div>
                    <div className="flex flex-col items-end gap-1">
                      {a.hasPat && (
                        <Badge variant="secondary" className="gap-1">
                          <KeyRound className="size-3" />
                          PAT
                        </Badge>
                      )}
                      {a.webhookInstalled && (
                        <Badge variant="success">webhook</Badge>
                      )}
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div
                    className="flex items-center gap-1 truncate text-xs text-muted-foreground"
                    title={a.repoUrl}
                  >
                    <ExternalLink className="size-3 shrink-0" />
                    <span className="truncate">{a.repoUrl}</span>
                  </div>
                  <div className="flex items-center justify-between text-xs text-muted-foreground">
                    <span>added {formatDate(a.createdAt)}</span>
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={(e) => {
                        e.preventDefault();
                        onDelete(a.id);
                      }}
                      aria-label="Remove agent"
                      className="opacity-0 transition-opacity group-hover:opacity-100"
                    >
                      <Trash2 />
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </Link>
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
          <Bot className="h-5 w-5 text-muted-foreground" />
        </div>
        <div>
          <p className="text-sm font-medium">No agents yet</p>
          <p className="text-sm text-muted-foreground">
            Add a git repository containing your agent code.
          </p>
        </div>
        <Button size="sm" asChild>
          <Link href="/agents/new">Add your first agent</Link>
        </Button>
      </CardContent>
    </Card>
  );
}
