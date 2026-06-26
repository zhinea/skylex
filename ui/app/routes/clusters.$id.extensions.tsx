import { ExtensionsCard } from "~/components/cluster/ExtensionsCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterExtensionsPage() {
  const { clusterId } = useClusterContext();
  return (
    <div className="space-y-6">
      <ExtensionsCard clusterId={clusterId} />
    </div>
  );
}
