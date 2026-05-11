"use client";

import { Layers, Lock, ScanFace, ShieldCheck } from "lucide-react";

// Static preview of the Environments concept. Not wired to storage yet —
// this page is the contract we show clients before the runtime work
// lands. When the storage / routing actually exists, replace the three
// demo tiles with live env records from the API.

const ENVS: { name: string; description: string; accent: string }[] = [
  {
    name: "DEV",
    description:
      "Auto-deploy on every push, smoke evals only, no approval gates.",
    accent: "border-foreground/60",
  },
  {
    name: "STAGING",
    description:
      "Full eval suite, optional approval, canary or progressive rollout.",
    accent: "border-amber-400/60",
  },
  {
    name: "PROD",
    description:
      "Strict policy gates, human approval, audit log, SLO-backed rollback.",
    accent: "border-rose-400/60",
  },
];

export default function EnvironmentsPage() {
  return (
    <div className="mx-auto w-full max-w-6xl space-y-6 p-6">
      <div>
        <div className="mb-1 flex items-center gap-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          <Layers className="size-3.5" />
          Environments
        </div>
        <h1 className="text-3xl font-semibold tracking-tight">Environments</h1>
        <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
          Each environment owns its runtime target, credentials, secrets,
          scaling, and approval policy. Pipelines reference envs by name;
          promotion moves an artifact from one env&rsquo;s pipeline to the
          next.
        </p>
      </div>

      <div className="rounded-lg border bg-card/40 p-5">
        {/* Three env tiles */}
        <div className="grid gap-4 md:grid-cols-3">
          {ENVS.map((e) => (
            <div
              key={e.name}
              className={`rounded-lg border-2 ${e.accent} bg-background/40 p-5`}
            >
              <div className="mb-2 flex items-start justify-between">
                <span className="font-mono text-sm font-semibold tracking-wider">
                  {e.name}
                </span>
                <span className="text-[11px] text-muted-foreground">tier</span>
              </div>
              <p className="text-xs text-muted-foreground">{e.description}</p>
            </div>
          ))}
        </div>

        {/* Concept rows */}
        <div className="mt-6 grid gap-5 border-t pt-5 md:grid-cols-3">
          <ConceptRow
            icon={ScanFace}
            title="Runtime target"
            body="K8s cluster, Bedrock AgentCore account, or Vertex Agent Engine project. Different per env."
          />
          <ConceptRow
            icon={Lock}
            title="Credentials"
            body="Cloud creds + registry auth, sealed at rest. Resolved by pipelines at run time."
          />
          <ConceptRow
            icon={ShieldCheck}
            title="Approval policy"
            body="Who can approve, by what method (UI / Slack / auto-policy / quorum), with timeout & escalation."
          />
        </div>

        {/* Footer */}
        <div className="mt-6 flex items-center justify-end border-t pt-5">
          <button
            type="button"
            disabled
            className="rounded-md border bg-foreground/95 px-3 py-1.5 text-xs font-medium text-background opacity-90 disabled:cursor-not-allowed"
            title="Coming soon"
          >
            + New environment
          </button>
        </div>
      </div>
    </div>
  );
}

function ConceptRow({
  icon: Icon,
  title,
  body,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  body: string;
}) {
  return (
    <div className="flex items-start gap-3">
      <div className="grid size-9 shrink-0 place-items-center rounded-md border bg-muted/30">
        <Icon className="size-4 text-muted-foreground" />
      </div>
      <div className="min-w-0">
        <div className="text-sm font-medium">{title}</div>
        <p className="mt-0.5 text-xs text-muted-foreground">{body}</p>
      </div>
    </div>
  );
}
