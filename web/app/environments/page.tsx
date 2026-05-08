import { EmptySection } from "@/components/empty-section";

export default function EnvironmentsPage() {
  return (
    <EmptySection
      title="Environments"
      description="Per-environment config (dev / staging / prod, runtime targets, secrets bindings) for agent deployments. Hooks into the Promote node."
    />
  );
}
