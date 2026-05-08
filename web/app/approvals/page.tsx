import { EmptySection } from "@/components/empty-section";

export default function ApprovalsPage() {
  return (
    <EmptySection
      title="Approvals"
      description="An inbox for pending Approval-node decisions across runs. The wiring exists (Restate awakeables + the Resume panel on each run page); this page will surface them in one place."
    />
  );
}
