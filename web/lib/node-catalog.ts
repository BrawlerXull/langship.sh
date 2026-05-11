// Catalog of node types the canvas can drop. These are the Langship CI/CD
// pipeline primitives — Trigger → Build → Test → Eval → Policy → Approval
// → Deploy → Promote → Rollback. The canonical type strings keep the
// `flow-nodes-base.` prefix so they line up with the executor registry in
// pkg/executors.
//
// Keep this in sync with pkg/executors/registry.go::RegisterAll(). Anything
// listed here without a matching executor will fail at run time with
// "executor not implemented for node type".

import type { ComponentType } from "react";
import {
  Play,
  Hammer,
  TestTube2,
  Gauge,
  ShieldCheck,
  ShieldAlert,
  Container,
  Pause,
  Rocket,
  ArrowUpFromLine,
  Undo2,
  CircleSlash,
  Settings2,
  UploadCloud,
} from "lucide-react";

export type CatalogEntry = {
  /** Runtime type string: flow-nodes-base.X */
  type: string;
  /** Display label */
  label: string;
  /** Short description shown in palette/inspector */
  description: string;
  /** Lucide icon */
  icon: ComponentType<{ className?: string }>;
  /** Tailwind colour class for the node header */
  color: string;
  /** Number of outputs (for source handles) */
  outputs: number;
  /** Default parameters when dropped */
  defaults: Record<string, unknown>;
  /** Default Settings (retry etc.) */
  settings?: Record<string, unknown>;
  /** Group in palette */
  group: "trigger" | "build" | "verify" | "gate" | "deploy" | "passthrough";
};

export const CATALOG: CatalogEntry[] = [
  {
    type: "flow-nodes-base.trigger",
    label: "Trigger",
    description: "Pipeline entry point. Every pipeline needs exactly one.",
    icon: Play,
    color: "bg-emerald-500",
    outputs: 1,
    defaults: { mode: "manual", fromBranch: "main", toBranch: "production" },
    group: "trigger",
  },
  {
    type: "flow-nodes-base.build",
    label: "Build",
    description: "Clone the agent repo and build an OCI image via BuildKit (or run a shell command).",
    icon: Hammer,
    color: "bg-amber-500",
    outputs: 1,
    defaults: {
      mode: "docker",
      dockerfile: "Dockerfile",
      context: ".",
      imageName: "",
      registry: "registry:5000",
      platform: "linux/amd64",
      buildArgs: "",
      // shell-mode fallback fields:
      command: "docker build -t $AGENT_NAME:$COMMIT_SHA .",
      workdir: ".",
      timeoutSeconds: 600,
    },
    settings: { retryOnFail: true, maxTries: 2, waitBetweenTries: 5000 },
    group: "build",
  },
  {
    type: "flow-nodes-base.test",
    label: "Test",
    description: "Run unit / integration tests against the build artifact.",
    icon: TestTube2,
    color: "bg-sky-500",
    outputs: 1,
    defaults: {
      command: "pytest -q",
      workdir: ".",
      timeoutSeconds: 600,
    },
    group: "verify",
  },
  {
    type: "flow-nodes-base.eval",
    label: "Eval",
    description: "Run agent evals (LLM benchmarks, scoring suites).",
    icon: Gauge,
    color: "bg-violet-500",
    outputs: 1,
    defaults: {
      suite: "default",
      threshold: 0.8,
      metric: "accuracy",
    },
    group: "verify",
  },
  {
    type: "flow-nodes-base.policy",
    label: "Policy",
    description: "Apply governance rules (budget, safety, compliance gates).",
    icon: ShieldCheck,
    color: "bg-indigo-500",
    outputs: 1,
    defaults: {
      rules: ["max_monthly_spend_usd:1000", "no_pii_in_outputs"],
      mode: "enforce",
    },
    group: "gate",
  },
  {
    type: "flow-nodes-base.waitForApproval",
    label: "Approval",
    description: "Pause until a human (or quorum) approves continuation.",
    icon: Pause,
    color: "bg-fuchsia-500",
    outputs: 1,
    defaults: {
      reason: "Manual review",
      reviewers: [],
    },
    group: "gate",
  },
  {
    type: "flow-nodes-base.imageScan",
    label: "Image Scan",
    description:
      "Scan the BUILT container image for CVEs, secrets, and base-image vulns. Pulls from the local registry and gates on severity.",
    icon: Container,
    color: "bg-fuchsia-600",
    outputs: 1,
    defaults: {
      tool: "trivy",
      severityThreshold: "HIGH",
      failOnFinding: true,
      timeoutSeconds: 600,
      insecure: true,
      registryUsername: "",
      registryPassword: "",
      // imageRef left empty → defaults to upstream __build.image
      // custom-only:
      image: "",
      command: "",
    },
    group: "gate",
  },
  {
    type: "flow-nodes-base.sast",
    label: "SAST",
    description:
      "Static analysis: scan the agent repo for vulnerabilities, secrets, and quality issues. Pluggable tool — Trivy / Semgrep / Gitleaks / SonarCloud / custom.",
    icon: ShieldAlert,
    color: "bg-rose-500",
    outputs: 1,
    defaults: {
      tool: "trivy",
      severityThreshold: "HIGH",
      failOnFinding: true,
      timeoutSeconds: 600,
      // sonar-only:
      sonarHost: "https://sonarcloud.io",
      organization: "",
      projectKey: "",
      sonarToken: "",
      branchName: "",
      // custom-only:
      image: "",
      command: "",
    },
    group: "gate",
  },
  {
    type: "flow-nodes-base.push",
    label: "Push",
    description:
      "Mirror the locally-built image to one or more external registries (GHCR, Docker Hub, ECR, etc.) in parallel.",
    icon: UploadCloud,
    color: "bg-cyan-500",
    outputs: 1,
    defaults: {
      // srcImage empty → defaults to upstream __build.image
      srcInsecure: true,
      targets: [
        {
          name: "ghcr",
          registry: "ghcr.io",
          image: "",
          tag: "",
          username: "",
          password: "",
          insecure: false,
        },
      ],
    },
    group: "deploy",
  },
  {
    type: "flow-nodes-base.deploy",
    label: "Deploy",
    description: "Deploy the built image to AWS Bedrock AgentCore (K8s + Vertex coming soon).",
    icon: Rocket,
    color: "bg-rose-500",
    outputs: 1,
    defaults: {
      target: "agentcore",
      credentialName: "aws",
      runtimeName: "",
      image: "",
      envVars: {},
      timeoutSeconds: 600,
    },
    group: "deploy",
  },
  {
    type: "flow-nodes-base.promote",
    label: "Promote",
    description: "Promote source by opening or merging a PR from fromBranch → toBranch on the agent's repo.",
    icon: ArrowUpFromLine,
    color: "bg-orange-500",
    outputs: 1,
    defaults: {
      mode: "open-pr",
      fromBranch: "",
      toBranch: "",
      title: "",
      body: "Promotion opened by Flow.",
      mergeMethod: "merge",
      timeoutSeconds: 60,
    },
    group: "deploy",
  },
  {
    type: "flow-nodes-base.rollback",
    label: "Rollback",
    description: "Revert to a previous deployed revision.",
    icon: Undo2,
    color: "bg-red-600",
    outputs: 1,
    defaults: {
      revision: "previous",
    },
    group: "deploy",
  },
  {
    type: "flow-nodes-base.set",
    label: "Set",
    description: "Define or transform fields on each item.",
    icon: Settings2,
    color: "bg-slate-500",
    outputs: 1,
    defaults: { values: { string: [] } },
    group: "passthrough",
  },
  {
    type: "flow-nodes-base.noOp",
    label: "No-op",
    description: "Pass items through unchanged. Useful as a placeholder.",
    icon: CircleSlash,
    color: "bg-slate-400",
    outputs: 1,
    defaults: {},
    group: "passthrough",
  },
];

export const GROUP_LABELS: Record<CatalogEntry["group"], string> = {
  trigger: "Trigger",
  build: "Build",
  verify: "Verify",
  gate: "Gate",
  deploy: "Deploy",
  passthrough: "Pass-through",
};

/** Look up by runtime type. Returns undefined for unsupported types so the
 *  caller can render a fallback "unsupported" node. */
export function lookup(type: string): CatalogEntry | undefined {
  return CATALOG.find((c) => c.type === type);
}

/** Suggest a unique name like "Build", "Build 2", "Build 3". */
export function uniqueName(base: string, existing: Set<string>): string {
  if (!existing.has(base)) return base;
  let i = 2;
  while (existing.has(`${base} ${i}`)) i++;
  return `${base} ${i}`;
}
