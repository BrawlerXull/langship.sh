"use client";

import { Activity, ArrowLeftRight, Coins, Gauge, Radio } from "lucide-react";

// Static preview of the AI Gateway concept. No real proxy yet — this
// page is the contract we show clients while the gateway is being
// built. When the runtime exists, replace the four feature tiles with
// live config + metrics.

export default function GatewayPage() {
  return (
    <div className="w-full space-y-6 p-6">
      <div>
        <div className="mb-1 flex items-center gap-2 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
          <Radio className="size-3.5" />
          Gateway
        </div>
        <h1 className="text-3xl font-semibold tracking-tight">AI Gateway</h1>
        <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
          A proxy in front of your agents&rsquo; LLM calls. Enforce
          per-agent budgets, log every prompt and completion for replay,
          rate-limit by tenant, and fall back across providers without
          redeploying.
        </p>
      </div>

      <div className="rounded-lg border bg-card/40 p-5">
        <div className="grid gap-4 md:grid-cols-2">
          <FeatureTile
            icon={Coins}
            title="Budgets & cost caps"
            body="Per-agent and per-environment spend ceilings. Hard cap or alert-only; the gateway short-circuits requests that would breach the budget."
          />
          <FeatureTile
            icon={Activity}
            title="Prompt + completion logs"
            body="Every call captured for replay and incident review. Plumbs into the Operations pillar's trace store."
          />
          <FeatureTile
            icon={Gauge}
            title="Rate limits & quotas"
            body="Throttle per tenant, per agent, or per route. Lives in the gateway, not the app — same policy across runtimes."
          />
          <FeatureTile
            icon={ArrowLeftRight}
            title="Provider fallback"
            body="Try OpenAI → Anthropic → Bedrock on failure. Routing is config, not code, so swaps don't need a redeploy."
          />
        </div>

        <div className="mt-6 flex items-center justify-end border-t pt-5">
          <button
            type="button"
            disabled
            className="rounded-md border bg-foreground/95 px-3 py-1.5 text-xs font-medium text-background opacity-90 disabled:cursor-not-allowed"
            title="Coming soon"
          >
            + New gateway
          </button>
        </div>
      </div>
    </div>
  );
}

function FeatureTile({
  icon: Icon,
  title,
  body,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  body: string;
}) {
  return (
    <div className="rounded-lg border bg-background/40 p-5">
      <div className="flex items-start gap-3">
        <div className="grid size-9 shrink-0 place-items-center rounded-md border bg-muted/30">
          <Icon className="size-4 text-muted-foreground" />
        </div>
        <div className="min-w-0">
          <div className="text-sm font-medium">{title}</div>
          <p className="mt-1 text-xs text-muted-foreground">{body}</p>
        </div>
      </div>
    </div>
  );
}
