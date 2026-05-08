"use client";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { PipelineNode } from "@/lib/pipeline-graph";

interface NodeFormProps {
  node: PipelineNode;
  onChange: (next: PipelineNode) => void;
}

// Typed forms per node type. Anything we don't know about renders a generic
// JSON view (handled by the inspector — this component returns null in that
// case so the parent shows the JSON fallback).
//
// Each form mutates node.parameters and calls onChange with the updated node.
// We deliberately keep these dumb (no internal state); the inspector owns
// debouncing and persistence.
export function NodeForm({ node, onChange }: NodeFormProps) {
  switch (node.type) {
    case "flow-nodes-base.trigger":
      return <TriggerForm node={node} onChange={onChange} />;
    case "flow-nodes-base.build":
      return <BuildForm node={node} onChange={onChange} />;
    case "flow-nodes-base.test":
      return <TestForm node={node} onChange={onChange} />;
    case "flow-nodes-base.eval":
      return <EvalForm node={node} onChange={onChange} />;
    case "flow-nodes-base.policy":
      return <PolicyForm node={node} onChange={onChange} />;
    case "flow-nodes-base.waitForApproval":
      return <ApprovalForm node={node} onChange={onChange} />;
    case "flow-nodes-base.push":
      return <PushForm node={node} onChange={onChange} />;
    case "flow-nodes-base.deploy":
      return <DeployForm node={node} onChange={onChange} />;
    case "flow-nodes-base.promote":
      return <PromoteForm node={node} onChange={onChange} />;
    case "flow-nodes-base.rollback":
      return <RollbackForm node={node} onChange={onChange} />;
    default:
      return null;
  }
}

/** Returns true if we render a typed form for this type (so the JSON
 *  fallback can be hidden). */
export function hasTypedForm(type: string): boolean {
  return [
    "flow-nodes-base.trigger",
    "flow-nodes-base.build",
    "flow-nodes-base.push",
    "flow-nodes-base.test",
    "flow-nodes-base.eval",
    "flow-nodes-base.policy",
    "flow-nodes-base.waitForApproval",
    "flow-nodes-base.deploy",
    "flow-nodes-base.promote",
    "flow-nodes-base.rollback",
  ].includes(type);
}

// --- helpers --------------------------------------------------------------

function setParam<T>(node: PipelineNode, key: string, value: T): PipelineNode {
  return {
    ...node,
    parameters: { ...(node.parameters ?? {}), [key]: value },
  };
}

function getString(node: PipelineNode, key: string, fallback = ""): string {
  const v = node.parameters?.[key];
  return typeof v === "string" ? v : fallback;
}

function getNumber(node: PipelineNode, key: string, fallback = 0): number {
  const v = node.parameters?.[key];
  return typeof v === "number" ? v : fallback;
}

function getStringArray(node: PipelineNode, key: string): string[] {
  const v = node.parameters?.[key];
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

function getBool(node: PipelineNode, key: string, fallback: boolean): boolean {
  const v = node.parameters?.[key];
  if (typeof v === "boolean") return v;
  return fallback;
}

// --- forms ----------------------------------------------------------------

function TriggerForm({ node, onChange }: NodeFormProps) {
  const mode = getString(node, "mode", "manual");
  const cron = getString(node, "cron", "0 * * * *");
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label>Mode</Label>
        <select
          value={mode}
          onChange={(e) => onChange(setParam(node, "mode", e.target.value))}
          className="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm"
        >
          <option value="manual">Manual</option>
          <option value="webhook">Git webhook (push)</option>
          <option value="schedule">Scheduled (cron)</option>
        </select>
        <p className="text-[11px] text-muted-foreground">
          How runs are dispatched. Webhook + manual are wired today; schedule
          lands once the cron worker exists.
        </p>
      </div>
      {mode === "schedule" && (
        <div className="space-y-1.5">
          <Label htmlFor="cron">Cron expression</Label>
          <Input
            id="cron"
            value={cron}
            onChange={(e) => onChange(setParam(node, "cron", e.target.value))}
            placeholder="0 * * * *"
            className="font-mono text-xs"
          />
        </div>
      )}
    </div>
  );
}

function BuildForm({ node, onChange }: NodeFormProps) {
  const mode = getString(node, "mode", "docker");
  const dockerfile = getString(node, "dockerfile", "Dockerfile");
  const ctx = getString(node, "context", ".");
  const imageName = getString(node, "imageName", "");
  const registry = getString(node, "registry", "registry:5000");
  const platform = getString(node, "platform", "linux/amd64");
  const buildArgs = getString(node, "buildArgs", "");

  const command = getString(
    node,
    "command",
    "docker build -t $AGENT_NAME:$COMMIT_SHA ."
  );
  const workdir = getString(node, "workdir", ".");
  const timeout = getNumber(node, "timeoutSeconds", 600);

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label>Mode</Label>
        <select
          value={mode}
          onChange={(e) => onChange(setParam(node, "mode", e.target.value))}
          className="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm"
        >
          <option value="docker">docker</option>
          <option value="shell">shell</option>
        </select>
        <p className="text-[11px] text-muted-foreground">
          <code>docker</code> = real OCI image build &amp; push via BuildKit.{" "}
          <code>shell</code> = run any command (escape hatch).
        </p>
      </div>

      {mode === "docker" ? (
        <>
          <div className="space-y-1.5">
            <Label htmlFor="dockerfile">Dockerfile</Label>
            <Input
              id="dockerfile"
              value={dockerfile}
              onChange={(e) =>
                onChange(setParam(node, "dockerfile", e.target.value))
              }
              placeholder="Dockerfile"
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              Path relative to the repo root.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="b-ctx">Build context</Label>
            <Input
              id="b-ctx"
              value={ctx}
              onChange={(e) => onChange(setParam(node, "context", e.target.value))}
              placeholder="."
              className="font-mono text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="b-image">Image name (optional)</Label>
            <Input
              id="b-image"
              value={imageName}
              onChange={(e) =>
                onChange(setParam(node, "imageName", e.target.value))
              }
              placeholder="my-org/my-agent"
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              Defaults to <code>&lt;owner&gt;/&lt;repo&gt;</code> from the
              agent&rsquo;s connection.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="b-reg">Registry</Label>
            <Input
              id="b-reg"
              value={registry}
              onChange={(e) =>
                onChange(setParam(node, "registry", e.target.value))
              }
              placeholder="ghcr.io"
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              In docker-compose, <code>registry:5000</code> is the bundled
              local registry (host port 5050 for <code>docker pull</code>).
              For <code>ghcr.io</code>, the agent&rsquo;s PAT must have{" "}
              <code>write:packages</code>.
            </p>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="b-plat">Target platform</Label>
            <Input
              id="b-plat"
              value={platform}
              onChange={(e) =>
                onChange(setParam(node, "platform", e.target.value))
              }
              placeholder="linux/amd64"
              className="font-mono text-xs"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="b-args">Build args (optional)</Label>
            <Input
              id="b-args"
              value={buildArgs}
              onChange={(e) =>
                onChange(setParam(node, "buildArgs", e.target.value))
              }
              placeholder="NODE_ENV=production, FOO=bar"
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              <code>key=value</code>, comma-separated.
            </p>
          </div>
        </>
      ) : (
        <>
          <div className="space-y-1.5">
            <Label htmlFor="command">Command</Label>
            <Textarea
              id="command"
              rows={3}
              value={command}
              onChange={(e) => onChange(setParam(node, "command", e.target.value))}
              spellCheck={false}
              className="font-mono text-xs"
            />
            <p className="text-[11px] text-muted-foreground">
              Runs in a clone of the agent repo. Available env:{" "}
              <code>$AGENT_NAME</code>, <code>$REPO_URL</code>,{" "}
              <code>$COMMIT_SHA</code>, <code>$REF</code>.
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="workdir">Workdir</Label>
            <Input
              id="workdir"
              value={workdir}
              onChange={(e) =>
                onChange(setParam(node, "workdir", e.target.value))
              }
              placeholder="."
              className="font-mono text-xs"
            />
          </div>
        </>
      )}

      <div className="space-y-1.5">
        <Label htmlFor="timeout">Timeout (seconds)</Label>
        <Input
          id="timeout"
          type="number"
          min={10}
          max={3600}
          value={timeout}
          onChange={(e) =>
            onChange(setParam(node, "timeoutSeconds", Number(e.target.value)))
          }
        />
      </div>
    </div>
  );
}

function TestForm({ node, onChange }: NodeFormProps) {
  const command = getString(node, "command", "pytest -q");
  const workdir = getString(node, "workdir", ".");
  const timeout = getNumber(node, "timeoutSeconds", 600);
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="t-cmd">Command</Label>
        <Textarea
          id="t-cmd"
          rows={3}
          value={command}
          onChange={(e) => onChange(setParam(node, "command", e.target.value))}
          spellCheck={false}
          className="font-mono text-xs"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="t-wd">Workdir</Label>
          <Input
            id="t-wd"
            value={workdir}
            onChange={(e) => onChange(setParam(node, "workdir", e.target.value))}
            placeholder="."
            className="font-mono text-xs"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="t-to">Timeout (s)</Label>
          <Input
            id="t-to"
            type="number"
            min={10}
            max={3600}
            value={timeout}
            onChange={(e) =>
              onChange(setParam(node, "timeoutSeconds", Number(e.target.value)))
            }
          />
        </div>
      </div>
      <p className="text-[11px] text-muted-foreground">
        Stub today: logs the command and returns success. Wire to a real
        executor when the test runner exists.
      </p>
    </div>
  );
}

function EvalForm({ node, onChange }: NodeFormProps) {
  const suite = getString(node, "suite", "default");
  const metric = getString(node, "metric", "accuracy");
  const threshold = getNumber(node, "threshold", 0.8);
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="e-suite">Suite</Label>
        <Input
          id="e-suite"
          value={suite}
          onChange={(e) => onChange(setParam(node, "suite", e.target.value))}
          placeholder="default"
        />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="e-metric">Metric</Label>
          <Input
            id="e-metric"
            value={metric}
            onChange={(e) => onChange(setParam(node, "metric", e.target.value))}
            placeholder="accuracy"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="e-threshold">Threshold</Label>
          <Input
            id="e-threshold"
            type="number"
            step="0.01"
            min={0}
            max={1}
            value={threshold}
            onChange={(e) =>
              onChange(setParam(node, "threshold", Number(e.target.value)))
            }
          />
        </div>
      </div>
    </div>
  );
}

function PolicyForm({ node, onChange }: NodeFormProps) {
  const rules = getStringArray(node, "rules");
  const mode = getString(node, "mode", "enforce");
  const text = rules.join("\n");

  function commit(t: string) {
    const parsed = t.split("\n").map((s) => s.trim()).filter(Boolean);
    onChange(setParam(node, "rules", parsed));
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label>Enforcement</Label>
        <select
          value={mode}
          onChange={(e) => onChange(setParam(node, "mode", e.target.value))}
          className="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm"
        >
          <option value="enforce">Enforce — fail on violation</option>
          <option value="warn">Warn — log only</option>
          <option value="audit">Audit — record, never block</option>
        </select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="p-rules">Rules (one per line)</Label>
        <Textarea
          id="p-rules"
          rows={6}
          defaultValue={text}
          onBlur={(e) => commit(e.target.value)}
          spellCheck={false}
          className="font-mono text-xs"
          placeholder={"max_monthly_spend_usd:1000\nno_pii_in_outputs"}
        />
      </div>
    </div>
  );
}

function ApprovalForm({ node, onChange }: NodeFormProps) {
  const reason = getString(node, "reason", "Manual review");
  const reviewers = getStringArray(node, "reviewers");
  const text = reviewers.join("\n");

  function commit(t: string) {
    const parsed = t.split("\n").map((s) => s.trim()).filter(Boolean);
    onChange(setParam(node, "reviewers", parsed));
  }

  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="a-reason">Reason</Label>
        <Input
          id="a-reason"
          value={reason}
          onChange={(e) => onChange(setParam(node, "reason", e.target.value))}
          placeholder="Manual review before deploy"
        />
        <p className="text-[11px] text-muted-foreground">
          Shown in the Resume panel on the executions page.
        </p>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="a-reviewers">Reviewers (one per line)</Label>
        <Textarea
          id="a-reviewers"
          rows={4}
          defaultValue={text}
          onBlur={(e) => commit(e.target.value)}
          spellCheck={false}
          className="font-mono text-xs"
          placeholder="user@example.com"
        />
      </div>
    </div>
  );
}

function PushForm({ node, onChange }: NodeFormProps) {
  const srcImage = getString(node, "srcImage", "");
  const targetRegistry = getString(node, "targetRegistry", "ghcr.io");
  const targetImage = getString(node, "targetImage", "");
  const tag = getString(node, "tag", "");
  const username = getString(node, "username", "");
  const password = getString(node, "password", "");
  const srcInsecure = getBool(node, "srcInsecure", true);
  const dstInsecure = getBool(node, "dstInsecure", false);
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="push-src">Source image (optional)</Label>
        <Input
          id="push-src"
          value={srcImage}
          onChange={(e) => onChange(setParam(node, "srcImage", e.target.value))}
          placeholder="registry:5000/owner/agent:sha (defaults to upstream Build output)"
          className="font-mono text-xs"
        />
        <p className="text-[11px] text-muted-foreground">
          Leave blank to use the upstream Build node&rsquo;s{" "}
          <code className="font-mono">__build.image</code>.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="push-reg">Target registry</Label>
          <Input
            id="push-reg"
            value={targetRegistry}
            onChange={(e) =>
              onChange(setParam(node, "targetRegistry", e.target.value))
            }
            placeholder="ghcr.io"
            className="font-mono text-xs"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="push-tag">Tag</Label>
          <Input
            id="push-tag"
            value={tag}
            onChange={(e) => onChange(setParam(node, "tag", e.target.value))}
            placeholder="defaults to commit SHA, then 'latest'"
            className="font-mono text-xs"
          />
        </div>
      </div>

      <div className="space-y-1.5">
        <Label htmlFor="push-img">Target image</Label>
        <Input
          id="push-img"
          value={targetImage}
          onChange={(e) => onChange(setParam(node, "targetImage", e.target.value))}
          placeholder="org/agent"
          className="font-mono text-xs"
        />
        <p className="text-[11px] text-muted-foreground">
          Final ref: <code className="font-mono">{targetRegistry || "<registry>"}/{targetImage || "<image>"}:{tag || "<tag>"}</code>
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="push-user">Username</Label>
          <Input
            id="push-user"
            value={username}
            onChange={(e) => onChange(setParam(node, "username", e.target.value))}
            placeholder="(empty = anonymous / docker config)"
            className="font-mono text-xs"
            autoComplete="off"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="push-pass">Password / token</Label>
          <Input
            id="push-pass"
            type="password"
            value={password}
            onChange={(e) => onChange(setParam(node, "password", e.target.value))}
            placeholder="ghp_… or registry password"
            className="font-mono text-xs"
            autoComplete="new-password"
          />
        </div>
      </div>

      <div className="rounded-md border bg-muted/20 p-2 text-[11px]">
        <div className="mb-1 font-medium uppercase tracking-wider text-muted-foreground">
          Insecure transport
        </div>
        <div className="flex items-center gap-2">
          <input
            id="push-src-insecure"
            type="checkbox"
            checked={srcInsecure}
            onChange={(e) =>
              onChange(setParam(node, "srcInsecure", e.target.checked))
            }
            className="size-3.5"
          />
          <Label htmlFor="push-src-insecure" className="text-[11px]">
            Source allows HTTP (default; the local{" "}
            <code className="font-mono">registry:5000</code> serves plain HTTP)
          </Label>
        </div>
        <div className="mt-1 flex items-center gap-2">
          <input
            id="push-dst-insecure"
            type="checkbox"
            checked={dstInsecure}
            onChange={(e) =>
              onChange(setParam(node, "dstInsecure", e.target.checked))
            }
            className="size-3.5"
          />
          <Label htmlFor="push-dst-insecure" className="text-[11px]">
            Destination allows HTTP (off — public registries are HTTPS)
          </Label>
        </div>
      </div>

      <p className="text-[11px] text-muted-foreground">
        Image is pulled from the source over OCI v2 and pushed to the target;
        no docker daemon needed. For GHCR, the password is a PAT with{" "}
        <code className="font-mono">write:packages</code>.
      </p>
    </div>
  );
}

function DeployForm({ node, onChange }: NodeFormProps) {
  const runtime = getString(node, "runtime", "kubernetes");
  const env = getString(node, "env", "dev");
  const target = getString(node, "target", "");
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label>Runtime</Label>
          <select
            value={runtime}
            onChange={(e) =>
              onChange(setParam(node, "runtime", e.target.value))
            }
            className="h-9 w-full rounded-md border border-input bg-transparent px-2 text-sm"
          >
            <option value="kubernetes">Kubernetes</option>
            <option value="bedrock">AWS Bedrock AgentCore</option>
            <option value="vertex">GCP Vertex Agent Engine</option>
          </select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="d-env">Environment</Label>
          <Input
            id="d-env"
            value={env}
            onChange={(e) => onChange(setParam(node, "env", e.target.value))}
            placeholder="dev"
          />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="d-target">Target</Label>
        <Input
          id="d-target"
          value={target}
          onChange={(e) => onChange(setParam(node, "target", e.target.value))}
          placeholder="cluster name / project / agent ID"
          className="font-mono text-xs"
        />
        <p className="text-[11px] text-muted-foreground">
          Runtime-specific. E.g. for K8s: cluster + namespace; for Vertex: GCP
          project + agent ID.
        </p>
      </div>
    </div>
  );
}

function PromoteForm({ node, onChange }: NodeFormProps) {
  const fromEnv = getString(node, "fromEnv", "staging");
  const toEnv = getString(node, "toEnv", "prod");
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1.5">
          <Label htmlFor="pr-from">From env</Label>
          <Input
            id="pr-from"
            value={fromEnv}
            onChange={(e) => onChange(setParam(node, "fromEnv", e.target.value))}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="pr-to">To env</Label>
          <Input
            id="pr-to"
            value={toEnv}
            onChange={(e) => onChange(setParam(node, "toEnv", e.target.value))}
          />
        </div>
      </div>
      <p className="text-[11px] text-muted-foreground">
        Promote follows the project&rsquo;s branching strategy. Stub today;
        will execute the real promotion (tag/branch/merge) when wired.
      </p>
    </div>
  );
}

function RollbackForm({ node, onChange }: NodeFormProps) {
  const revision = getString(node, "revision", "previous");
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <Label htmlFor="rb-rev">Revision</Label>
        <Input
          id="rb-rev"
          value={revision}
          onChange={(e) => onChange(setParam(node, "revision", e.target.value))}
          placeholder='"previous" or a specific build ID'
          className="font-mono text-xs"
        />
      </div>
    </div>
  );
}
