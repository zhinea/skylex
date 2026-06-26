import { ManagedRolesCard } from "~/components/cluster/ManagedRolesCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterRolesPage() {
  const { clusterId, displayHost, displayPort, sslMode, revealedRole, setRevealedRole } = useClusterContext();
  return (
    <div className="space-y-6">
      <ManagedRolesCard
        clusterId={clusterId}
        host={displayHost}
        port={displayPort}
        sslMode={sslMode}
        revealed={revealedRole}
        onReveal={setRevealedRole}
        onDismissReveal={() => setRevealedRole(null)}
      />
    </div>
  );
}
