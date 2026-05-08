import { EmptySection } from "@/components/empty-section";

export default function GatewayPage() {
  return (
    <EmptySection
      title="Gateway"
      description="Public ingress for deployed agents — routes, auth, rate limits, OpenTelemetry export. Replaces the per-runtime gateway (Bedrock AgentCore endpoints, Vertex Reasoning Engine routes, K8s Ingress) with a uniform layer."
    />
  );
}
