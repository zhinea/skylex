import { SettingsCard } from "~/components/cluster/SettingsCard";
import { useClusterContext } from "./clusters.$id";

export default function ClusterSettingsPage() {
  const { clusterId, cluster } = useClusterContext();
  return (
    <div className="space-y-6">
      <SettingsCard clusterId={clusterId} cluster={cluster} />
    </div>
  );
}
