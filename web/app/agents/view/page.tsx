"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import {
  ArrowLeft,
  CheckCircle2,
  ExternalLink,
  Github,
  KeyRound,
  Lock,
  Play,
  Plus,
  Trash2,
  Webhook,
  XCircle,
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
import {
  CredentialForm,
  CredentialRow,
} from "@/components/credentials/credential-form";
import {
  api,
  type Agent,
  type AuthStatus,
  type FlowSummary,
  type PublicCredential,
  type Run,
  type ServerConfig,
} from "@/lib/api";
import { formatDate } from "@/lib/utils";

export default function AgentDetailPage() {
  return (
    <Suspense fallback={<div className="p-6 text-sm text-muted-foreground">Loading…</div>}>
      <AgentDetail />
    </Suspense>
  );
}

function AgentDetail() {
  const router = useRouter();
  const params = useSearchParams();
  const id = params.get("id") ?? "";

  const [agent, setAgent] = useState<Agent | null>(null);
  const [config, setConfig] = useState<ServerConfig | null>(null);
  const [pipelines, setPipelines] = useState<FlowSummary[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null); // which action is in flight
  const [showPipelinePicker, setShowPipelinePicker] = useState(false);

  async function load() {
    if (!id) return;
    try {
      const [a, cfg, allPipes] = await Promise.all([
        api.getAgent(id),
        api.getConfig().catch(() => null),
        api.listFlows().catch(() => []),
      ]);
      setAgent(a);
      setConfig(cfg);
      setPipelines(allPipes);
      // Pull recent runs across all attached pipelines.
      if (a.attachedPipelines?.length) {
        const lists = await Promise.all(
          a.attachedPipelines.map((pid) =>
            api.listRuns({ pipelineId: pid, limit: 5 }).catch(() => [])
          )
        );
        const merged = lists.flat();
        merged.sort((x, y) => (y.startedAt || "").localeCompare(x.startedAt || ""));
        setRuns(merged.slice(0, 10));
      } else {
        setRuns([]);
      }
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : "load failed");
    }
  }

  useEffect(() => {
    load();
    // Reload recent runs whenever any run is dispatched (manual / agent /
    // GitHub push). Cheap — `load()` is one round-trip.
    const es = new EventSource(api.runsStreamURL());
    es.onmessage = () => load();
    es.onerror = () => {};
    return () => es.close();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id]);

  async function withBusy<T>(label: string, fn: () => Promise<T>) {
    setBusy(label);
    setError(null);
    try {
      return await fn();
    } catch (e) {
      setError(e instanceof Error ? e.message : `${label} failed`);
    } finally {
      setBusy(null);
    }
  }

  async function onTrigger() {
    await withBusy("trigger", async () => {
      const res = await api.triggerAgent(id);
      const ids = res.executionIds ?? [];
      if (ids.length === 1) {
        router.push(`/executions/view/?id=${encodeURIComponent(ids[0])}`);
        return;
      }
      if (ids.length > 1) {
        router.push(`/executions/multi/?ids=${ids.map(encodeURIComponent).join(",")}`);
        return;
      }
      // Nothing dispatched — surface failures so the user sees why.
      const fails = res.failures ?? [];
      if (fails.length === 0) {
        throw new Error("trigger returned no executions and no failure detail");
      }
      throw new Error(
        fails
          .map((f) => `${f.pipelineId}: ${f.reason}${f.error ? " — " + f.error : ""}`)
          .join("; ")
      );
    });
  }

  async function onTestAuth() {
    await withBusy("test-auth", async () => {
      await api.testAgentAuth(id);
      await load();
    });
  }

  async function onInstallWebhook() {
    await withBusy("webhook-install", async () => {
      await api.installAgentWebhook(id);
      await load();
    });
  }

  async function onUninstallWebhook() {
    if (!confirm("Uninstall the GitHub webhook for this agent?")) return;
    await withBusy("webhook-uninstall", async () => {
      await api.uninstallAgentWebhook(id);
      await load();
    });
  }

  async function onAttachPipeline(pipelineId: string) {
    await withBusy("attach", async () => {
      await api.attachPipeline(id, pipelineId);
      setShowPipelinePicker(false);
      await load();
    });
  }

  async function onDetachPipeline(pipelineId: string) {
    if (!confirm("Detach this pipeline from the agent?")) return;
    await withBusy("detach", async () => {
      await api.detachPipeline(id, pipelineId);
      await load();
    });
  }

  async function onDeleteAgent() {
    if (!confirm("Delete this agent? Webhook will be removed too.")) return;
    await withBusy("delete", async () => {
      await api.deleteAgent(id);
      router.push("/agents");
    });
  }

  if (!id) {
    return (
      <div className="p-6 text-sm text-muted-foreground">
        Missing <code>id</code> query param.
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="p-6 space-y-3">
        {error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : (
          <p className="text-sm text-muted-foreground">Loading agent…</p>
        )}
      </div>
    );
  }

  const lastRun = runs[0];
  const attachedPipelineDetails = (agent.attachedPipelines ?? [])
    .map((pid) => pipelines.find((p) => p.id === pid))
    .filter((p): p is FlowSummary => Boolean(p));
  const attachable = pipelines.filter(
    (p) => !agent.attachedPipelines?.includes(p.id)
  );

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/agents">
            <ArrowLeft />
            Back
          </Link>
        </Button>
      </div>

      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-3xl font-semibold tracking-tight">{agent.name}</h1>
          <p className="mt-1 font-mono text-xs text-muted-foreground">{agent.id}</p>
        </div>
        <Button
          variant="destructive"
          onClick={onDeleteAgent}
          disabled={busy === "delete"}
        >
          <Trash2 />
          Delete
        </Button>
      </div>

      {error && (
        <Card className="border-destructive/40">
          <CardContent className="pt-6 text-sm text-destructive">{error}</CardContent>
        </Card>
      )}

      {/* Overview ---------------------------------------------------------- */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
          <CardTitle>Overview</CardTitle>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" asChild>
              <a href={agent.repoUrl} target="_blank" rel="noreferrer">
                <Github />
                Repository
              </a>
            </Button>
            <Button
              size="sm"
              onClick={onTrigger}
              disabled={
                busy === "trigger" ||
                !config?.orchestratorEnabled ||
                !agent.attachedPipelines?.length
              }
              title={
                !config?.orchestratorEnabled
                  ? "Orchestrator not configured (Restate unreachable)"
                  : !agent.attachedPipelines?.length
                    ? "Attach a pipeline first"
                    : busy === "trigger"
                      ? "Dispatching…"
                      : "Trigger a run on every attached pipeline"
              }
            >
              <Play />
              {busy === "trigger" ? "Triggering…" : "Trigger run"}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
            <Field label="Repository">
              <div className="flex items-center gap-2">
                <Github className="size-4 text-muted-foreground" />
                <span className="font-mono text-sm">{agent.name}</span>
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-1.5">
                <WebhookBadge agent={agent} />
                <AuthBadge status={agent.authStatus ?? "untested"} />
              </div>
            </Field>
            <Field label="Last run">
              {lastRun ? (
                <Link
                  href={`/executions/view/?id=${encodeURIComponent(lastRun.id)}`}
                  className="text-sm hover:underline"
                >
                  <RunStatus status={lastRun.status} />
                  <div className="mt-0.5 text-xs text-muted-foreground">
                    {formatDate(lastRun.startedAt)}
                  </div>
                </Link>
              ) : (
                <span className="text-sm text-muted-foreground">No runs yet.</span>
              )}
            </Field>
            <Field label="Pipelines">
              {attachedPipelineDetails.length === 0 ? (
                <span className="text-sm text-muted-foreground">
                  None attached.{" "}
                  <Link href="/flows/new" className="underline hover:text-foreground">
                    Create one
                  </Link>
                  .
                </span>
              ) : (
                <span className="text-sm">
                  {attachedPipelineDetails.length} attached
                </span>
              )}
            </Field>
          </div>

          {agent.attachedPipelines?.length ? (
            <p className="text-sm text-muted-foreground">
              Pushes to{" "}
              <code className="font-mono text-xs">{agent.name}</code> route through
              this agent&rsquo;s pipelines (matched by branch).
            </p>
          ) : null}

          {agent.webhookUrl && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Webhook className="size-3.5" />
              <code className="break-all font-mono">{agent.webhookUrl}</code>
            </p>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Repo / webhook -------------------------------------------------- */}
        <Card>
          <CardHeader>
            <CardTitle>Repo</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <div className="font-mono text-sm">{agent.name}</div>
              <div className="text-xs text-muted-foreground">
                Token{" "}
                {agent.hasPat ? (
                  <span className="font-mono">********</span>
                ) : (
                  <span>not set</span>
                )}
              </div>
            </div>

            <div className="flex items-center gap-2 text-sm">
              <span className="text-muted-foreground">Auth:</span>
              <AuthBadge status={agent.authStatus ?? "untested"} />
              {agent.authCheckedAt && (
                <span className="text-xs text-muted-foreground">
                  · {formatDate(agent.authCheckedAt)}
                </span>
              )}
            </div>

            <div className="flex items-center gap-2 text-sm">
              <span className="text-muted-foreground">Webhook:</span>
              <WebhookBadge agent={agent} />
              {agent.webhookInstalledAt && (
                <span className="text-xs text-muted-foreground">
                  · {formatDate(agent.webhookInstalledAt)}
                </span>
              )}
            </div>

            {agent.webhookUrl && (
              <p className="break-all rounded-md border bg-muted/30 p-2 font-mono text-[11px]">
                {agent.webhookUrl}
              </p>
            )}

            {!config?.webhooksAvailable && !agent.webhookInstalled && (
              <p className="rounded-md border border-amber-500/40 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-400">
                FLOW_PUBLIC_URL is not configured on the server. Set it (e.g. to
                a <code className="font-mono">cloudflared</code> tunnel) to install
                webhooks.
              </p>
            )}

            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={onTestAuth}
                disabled={!agent.hasPat || busy === "test-auth"}
              >
                {busy === "test-auth" ? "Testing…" : "Test auth"}
              </Button>
              {agent.webhookInstalled ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={onUninstallWebhook}
                  disabled={busy === "webhook-uninstall"}
                >
                  {busy === "webhook-uninstall" ? "Removing…" : "Uninstall webhook"}
                </Button>
              ) : (
                <Button
                  size="sm"
                  onClick={onInstallWebhook}
                  disabled={
                    !agent.hasPat ||
                    !config?.webhooksAvailable ||
                    busy === "webhook-install"
                  }
                >
                  <Webhook />
                  {busy === "webhook-install" ? "Installing…" : "Install webhook"}
                </Button>
              )}
            </div>
          </CardContent>
        </Card>

        {/* Pipelines ----------------------------------------------------- */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0">
            <CardTitle>Pipelines</CardTitle>
            {attachable.length > 0 ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setShowPipelinePicker((v) => !v)}
              >
                <Plus />
                Add pipeline
              </Button>
            ) : (
              <Button size="sm" variant="ghost" disabled>
                No more to add
              </Button>
            )}
          </CardHeader>
          <CardContent className="space-y-3">
            {attachedPipelineDetails.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No pipelines attached. Click &ldquo;Add pipeline&rdquo; to bind one
                (or create one in{" "}
                <Link href="/flows/new" className="underline">
                  /flows/new
                </Link>
                ).
              </p>
            ) : (
              <ul className="space-y-1.5">
                {attachedPipelineDetails.map((p) => (
                  <li
                    key={p.id}
                    className="flex items-center justify-between gap-2 rounded-md border bg-muted/20 px-3 py-2"
                  >
                    <div className="min-w-0">
                      <Link
                        href={`/flows/view/?id=${encodeURIComponent(p.id)}`}
                        className="truncate text-sm font-medium hover:underline"
                      >
                        {p.name || "Untitled"}
                      </Link>
                      <div className="text-[11px] text-muted-foreground">
                        {p.nodeCount} nodes · {formatDate(p.updatedAt)}
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label="Detach pipeline"
                      onClick={() => onDetachPipeline(p.id)}
                    >
                      <XCircle className="size-4" />
                    </Button>
                  </li>
                ))}
              </ul>
            )}

            {showPipelinePicker && attachable.length > 0 && (
              <div className="rounded-md border bg-background p-2">
                <div className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                  Attach a pipeline
                </div>
                <ul className="space-y-1">
                  {attachable.map((p) => (
                    <li key={p.id}>
                      <button
                        type="button"
                        onClick={() => onAttachPipeline(p.id)}
                        disabled={busy === "attach"}
                        className="flex w-full items-center justify-between rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent"
                      >
                        <span className="truncate">{p.name || "Untitled"}</span>
                        <span className="text-[11px] text-muted-foreground">
                          {p.nodeCount} nodes
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Credentials ------------------------------------------------------ */}
      <CredentialsSection
        agentId={id}
        credentials={agent.credentials ?? []}
        onChanged={async () => {
          // refetch agent so the credentials list updates
          const a = await api.getAgent(id);
          setAgent(a);
        }}
      />

      {/* Recent runs ------------------------------------------------------ */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0">
          <CardTitle>Recent runs</CardTitle>
          {runs.length > 0 && (
            <Link
              href="/executions/view"
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              View all →
            </Link>
          )}
        </CardHeader>
        <CardContent>
          {runs.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No runs yet — trigger one from the Overview block above.
            </p>
          ) : (
            <ul className="divide-y">
              {runs.map((r) => (
                <li key={r.id} className="flex items-center justify-between py-2">
                  <Link
                    href={`/executions/view/?id=${encodeURIComponent(r.id)}`}
                    className="min-w-0 flex-1"
                  >
                    <div className="flex items-center gap-2">
                      <RunStatus status={r.status} />
                      <span className="truncate font-mono text-xs">{r.id}</span>
                    </div>
                    <div className="text-[11px] text-muted-foreground">
                      {r.pipelineName || r.pipelineId} ·{" "}
                      {formatDate(r.startedAt)}
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="mb-1 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </div>
      {children}
    </div>
  );
}

function AuthBadge({ status }: { status: AuthStatus }) {
  if (status === "ok") {
    return (
      <Badge variant="success" className="gap-1">
        <CheckCircle2 className="size-3" />
        auth ok
      </Badge>
    );
  }
  if (status === "failed") {
    return (
      <Badge variant="destructive" className="gap-1">
        <XCircle className="size-3" />
        auth failed
      </Badge>
    );
  }
  return <Badge variant="outline">auth untested</Badge>;
}

function WebhookBadge({ agent }: { agent: Agent }) {
  if (agent.webhookInstalled) {
    return (
      <Badge variant="success" className="gap-1">
        <Webhook className="size-3" />
        webhook installed
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1">
      <Webhook className="size-3" />
      no webhook
    </Badge>
  );
}

function RunStatus({ status }: { status: string }) {
  const s = status.toLowerCase();
  if (s === "success" || s === "completed") {
    return (
      <Badge variant="success" className="gap-1">
        <CheckCircle2 className="size-3" />
        {status}
      </Badge>
    );
  }
  if (s === "failed" || s === "error") {
    return (
      <Badge variant="destructive" className="gap-1">
        <XCircle className="size-3" />
        {status}
      </Badge>
    );
  }
  return <Badge variant="secondary">{status}</Badge>;
}

// Avoid unused-import lint when the symbol is referenced only by type.
void KeyRound;
void ExternalLink;


// ─── Credentials ────────────────────────────────────────────────────────────

type CredentialsSectionProps = {
  agentId: string;
  credentials: PublicCredential[];
  onChanged: () => void | Promise<void>;
};

function CredentialsSection({ agentId, credentials, onChanged }: CredentialsSectionProps) {
  const [adding, setAdding] = useState(false);
  const [editingName, setEditingName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [globals, setGlobals] = useState<PublicCredential[]>([]);

  // Pull globals once so we can show inherited rows alongside the
  // per-agent overrides. Refreshed when `onChanged` re-fetches the agent
  // (cheap — credentials list is small).
  useEffect(() => {
    let cancelled = false;
    api.listGlobalCredentials().then((g) => {
      if (!cancelled) setGlobals(g);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, [credentials]);

  // Globals shadowed by an agent override: hide them from the inherited
  // list; the override row is the source of truth.
  const overrideNames = new Set(credentials.map((c) => c.name.toLowerCase()));
  const inherited = globals.filter((g) => !overrideNames.has(g.name.toLowerCase()));

  async function handleDelete(name: string) {
    if (!confirm(`Delete agent override "${name}"? The pipeline will fall back to the global credential of the same name (if any).`)) return;
    try {
      await api.deleteCredential(agentId, name);
      await onChanged();
    } catch (e) {
      setError(e instanceof Error ? e.message : "delete failed");
    }
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <div>
          <CardTitle className="flex items-center gap-2">
            <Lock className="size-4" />
            Credentials
          </CardTitle>
          <CardDescription>
            Cloud creds available to nodes for this agent. Globals defined on
            the <Link href="/credentials" className="underline">Credentials page</Link> are
            inherited; add an override here to specialize a credential for
            this agent only.
          </CardDescription>
        </div>
        <Button size="sm" onClick={() => { setAdding(true); setEditingName(null); }}>
          <Plus />
          Add override
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {error && (
          <p className="rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm text-destructive">
            {error}
          </p>
        )}

        {!adding && credentials.length === 0 && inherited.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No credentials available. Add a global one on the{" "}
            <Link href="/credentials" className="underline">Credentials page</Link>{" "}
            or an agent-specific override here.
          </p>
        )}

        {credentials.map((c) => (
          <div key={c.id} className="rounded-md border bg-muted/20 p-3">
            {editingName === c.name ? (
              <CredentialForm
                initial={c}
                onCancel={() => setEditingName(null)}
                onSubmit={async (body) => {
                  await api.updateCredential(agentId, c.name, body);
                  setEditingName(null);
                  await onChanged();
                }}
                onError={setError}
              />
            ) : (
              <CredentialRow
                cred={c}
                scopeLabel="agent override"
                onEdit={() => setEditingName(c.name)}
                onDelete={() => handleDelete(c.name)}
              />
            )}
          </div>
        ))}

        {inherited.map((c) => (
          <div key={`g-${c.id}`} className="rounded-md border border-dashed bg-muted/10 p-3 opacity-90">
            <CredentialRow
              cred={c}
              scopeLabel="inherited (global)"
              onEdit={() => { /* edit globals on the global page */ }}
              onDelete={() => { /* deletes go through global page */ }}
            />
          </div>
        ))}

        {adding && (
          <div className="rounded-md border bg-muted/20 p-3">
            <CredentialForm
              initial={null}
              onCancel={() => setAdding(false)}
              onSubmit={async (body) => {
                await api.createCredential(agentId, body);
                setAdding(false);
                await onChanged();
              }}
              onError={setError}
            />
          </div>
        )}
      </CardContent>
    </Card>
  );
}
