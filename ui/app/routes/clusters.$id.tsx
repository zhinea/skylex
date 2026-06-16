import { useParams, Link } from "react-router";
import { useCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: nodesData } = useNodes(id || "");

  if (clusterLoading) return <PageSpinner />;

  const cluster = clusterData?.cluster;
  if (!cluster) {
    return (
      <div className="text-center py-12">
        <h3 className="text-lg font-medium text-gray-900 dark:text-white">Cluster not found</h3>
        <Link to="/clusters" className="mt-4 text-blue-600 hover:text-blue-800 text-sm">Back to Clusters</Link>
      </div>
    );
  }

  const nodes = nodesData?.nodes || [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <Link to="/clusters" className="text-sm text-blue-600 hover:text-blue-800 dark:text-blue-400 mb-1 block">
            ← Back to Clusters
          </Link>
          <h2 className="text-2xl font-bold text-gray-900 dark:text-white">{cluster.name}</h2>
        </div>
        <Badge label={cluster.status} />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <Card title="Configuration">
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Engine</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.engine || "POSTGRESQL"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Version</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.version || "16"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Replication</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.replicationMode || "ASYNC"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Replicas</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.replicaCount || 0}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">PITR</dt>
              <dd className="text-gray-900 dark:text-white">{cluster.config?.pitrEnabled ? "Enabled" : "Disabled"}</dd>
            </div>
            <div className="flex justify-between">
              <dt className="text-gray-500 dark:text-gray-400">Created</dt>
              <dd className="text-gray-900 dark:text-white">{new Date(cluster.createdAt).toLocaleString()}</dd>
            </div>
          </dl>
        </Card>

        <Card title="Labels">
          {cluster.config?.labels && Object.keys(cluster.config.labels).length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {Object.entries(cluster.config.labels).map(([k, v]) => (
                <span key={k} className="px-2 py-1 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded text-xs">
                  {k}: {v}
                </span>
              ))}
            </div>
          ) : (
            <p className="text-sm text-gray-500 dark:text-gray-400">No labels configured</p>
          )}
        </Card>
      </div>

      <Card title={`Nodes (${nodes.length})`}>
        {nodes.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No nodes registered for this cluster.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Hostname</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Address</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Version</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Last Seen</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => (
                  <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-4 py-3 text-gray-900 dark:text-white font-medium">{n.hostname}</td>
                    <td className="px-4 py-3"><Badge label={n.role} /></td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{n.address}:{n.port}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{n.agentVersion || "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400">
                      {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
    </div>
  );
}