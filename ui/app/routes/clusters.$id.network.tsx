import { NetworkAccessCard } from "~/components/cluster/NetworkAccessCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterNetworkPage() {
  const { clusterId, nodes } = useClusterContext();
  return (
    <div className="space-y-6">
      <NetworkAccessCard clusterId={clusterId} nodes={nodes} />
    </div>
  );
}
