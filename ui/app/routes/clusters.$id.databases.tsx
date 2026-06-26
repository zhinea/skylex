import { ManagedDatabasesCard } from "~/components/cluster/ManagedDatabasesCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterDatabasesPage() {
  const { clusterId, displayHost, displayPort, sslMode, revealedRole } = useClusterContext();
  return (
    <div className="space-y-6">
      <ManagedDatabasesCard
        clusterId={clusterId}
        host={displayHost}
        port={displayPort}
        sslMode={sslMode}
        revealedRole={revealedRole}
      />
    </div>
  );
}
