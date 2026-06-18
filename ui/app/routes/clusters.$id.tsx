import { useEffect, useRef } from "react";
import { useParams, Link } from "react-router";
import { useCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { useCommandLogs } from "~/hooks/useCommandLogs";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";

function PgStatusBadges({
  installed,
  version,
  dataInitialized,
}: {
  installed: boolean;
  version: string;
  dataInitialized: boolean;
}) {
  if (!installed) {
    return (
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200">
        not installed
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        {version || "installed"}
      </span>
      <span
        className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
          dataInitialized
            ? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
            : "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
        }`}
      >
        {dataInitialized ? "data ready" : "not initialized"}
      </span>
    </span>
  );
}

export default function ClusterDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: clusterData, isLoading: clusterLoading } = useCluster(id || "");
  const { data: nodesData } = useNodes(id || "");
  const { data: logsData } = useCommandLogs(id || "");
  const logsEndRef = useRef<HTMLDivElement>(null);

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
  const missingPgNodes = nodes.filter((n) => !n.postgresInstalled);

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

      {missingPgNodes.length > 0 && (
        <div className="mb-4 flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-700">
          <span className="text-yellow-600 dark:text-yellow-400 mt-0.5">⚠</span>
          <p className="text-sm text-yellow-800 dark:text-yellow-200">
            {missingPgNodes.length === 1
              ? `Node "${missingPgNodes[0].hostname}" does not have PostgreSQL installed.`
              : `${missingPgNodes.length} nodes in this cluster do not have PostgreSQL installed.`}{" "}
            Install PostgreSQL on those hosts before promoting or replicating.
          </p>
        </div>
      )}

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
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">PostgreSQL</th>
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
                    <td className="px-4 py-3">
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </td>
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

      <div className="mt-6">
        <Card title="Command Logs">
          {logs.length === 0 ? (
            <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
              No command logs yet. Logs appear while the agent executes commands.
            </p>
          ) : (
            <div className="overflow-x-auto max-h-96 overflow-y-auto font-mono text-xs">
              <table className="w-full">
                <tbody>
                  {logs.map((log) => (
                    <tr key={log.id} className="border-b border-gray-100 dark:border-gray-800">
                      <td className="px-2 py-1.5 whitespace-nowrap text-gray-500 dark:text-gray-400">
                        {new Date(log.timestampMs).toLocaleTimeString()}
                      </td>
                      <td className="px-2 py-1.5 whitespace-nowrap text-gray-700 dark:text-gray-300">
                        {log.hostname || log.nodeId?.slice(0, 8) || "-"}
                      </td>
                      <td className="px-2 py-1.5 whitespace-nowrap">
                        <span className={levelColor(log.level)}>{log.level.toUpperCase()}</span>
                      </td>
                      <td className="px-2 py-1.5 text-gray-900 dark:text-white break-all">
                        {log.message}
                      </td>
                    </tr>
                  ))}
                  <tr><td colSpan={4}><div ref={logsEndRef} /></td></tr>
                </tbody>
              </table>
            </div>
          )}
        </Card>
      </div>
    </div>
  );
}