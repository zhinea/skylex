import { TLSConfigCard } from "~/components/cluster/TLSConfigCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterTLSPage() {
  const { clusterId, nodes } = useClusterContext();
  return (
    <div className="space-y-6">
      <TLSConfigCard clusterId={clusterId} nodes={nodes} />
    </div>
  );
}
