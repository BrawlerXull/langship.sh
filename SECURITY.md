# Security Policy

## Reporting a vulnerability

**Do not open a public GitHub issue for security problems.**

Email **<khush@lyzr.ai>** with:

- a description of the issue and its impact,
- steps to reproduce (a minimal PoC if you have one),
- affected version / commit,
- any suggested fix.

You'll get an acknowledgement within a few business days. We'll work with you on
a fix and a disclosure timeline; please give us reasonable time to ship a patch
before disclosing publicly.

## Scope

Langship is **self-hosted** — there's no hosted service to attack. Reports we
care most about:

- **Secret handling** — anything that could expose AES-GCM-sealed credentials,
  `FLOW_SECRET_KEY`, agent PATs, GitHub webhook secrets, or cloud creds; weak
  sealing; secrets leaking into logs / SSE streams / API responses.
- **Auth / access control** — bypassing intended access to agents, environments,
  credentials, runs, or the resume/approval endpoints.
- **Webhook receiver** — `/webhooks/github/{id}` HMAC verification bypass or
  forged-payload dispatch.
- **SSRF / injection** — via repo URLs, registry hosts, Build's `mode: shell`
  command, scanner sibling-container args, or the Restate ingress URL.
- **AWS deploy path** (`pkg/awsdeploy`) — privilege escalation via the
  cross-account AssumeRole, the generated IAM role/policy, or the AgentCore
  control-plane calls.
- **Supply chain** — dependency confusion / typosquatting affecting `flow`, the
  `web/` bundle, or `langship-cli/`.

Out of scope: issues that require an already-compromised host or a malicious
operator (the operator is fully trusted by design — they run the whole stack),
and findings against third-party services Langship merely talks to (GitHub, AWS,
your registry).

## Hardening notes for operators

- Set a strong, **persisted** `FLOW_SECRET_KEY` — losing it makes sealed
  credentials unrecoverable; leaking it exposes them all.
- Don't expose the API, Restate admin/ingress, BuildKit, or MinIO to the public
  internet — put them behind your own auth / network policy. `FLOW_PUBLIC_URL`
  only needs the webhook path reachable.
- The agent PAT only needs `repo` read (and `write:packages` if Build pushes to
  GHCR); scope it minimally.
- The AWS cross-account role used by Deploy needs ECR + IAM + bedrock-agentcore
  permissions — scope its trust policy to your Langship host's identity only.
