import { PostgreSQLConnectionCard } from "~/components/cluster/PostgreSQLConnectionCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterConnectionPage() {
  const { clusterId, nodes, cluster } = useClusterContext();
  return (
    <div className="space-y-6">
      {nodes.length > 0 ? (
        <PostgreSQLConnectionCard clusterId={clusterId} nodes={nodes} cluster={cluster} />
      ) : (
        <p className="text-xs text-muted-foreground py-8 text-center bg-card border rounded-xl shadow-xs">
          No nodes configured. Add nodes to view connection details.
        </p>
      )}
    </div>
  );
}
