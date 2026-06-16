import { useState } from "react";
import { useNodes, useDrainNode } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";

export default function NodesPage() {
  const [page, setPage] = useState(1);
  const { data, isLoading } = useNodes(undefined, page);
  const drainNode = useDrainNode();
  const [drainId, setDrainId] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const nodes = data?.nodes || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  return (
    <div>
      <h2 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">Nodes</h2>
      <Card>
        {nodes.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No nodes registered. Deploy agents to manage your database servers.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Hostname</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Cluster</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Address</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Agent Version</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Last Seen</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {nodes.map((n) => (
                  <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-750">
                    <td className="px-4 py-3 text-gray-900 dark:text-white font-medium">{n.hostname}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">
                      {n.clusterId ? <span className="text-xs text-gray-500">{n.clusterId.substring(0, 8)}...</span> : "-"}
                    </td>
                    <td className="px-4 py-3"><Badge label={n.role} /></td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{n.address}:{n.port}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{n.agentVersion || "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">
                      {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => setDrainId(n.id)}
                        className="text-xs text-red-600 hover:text-red-800 dark:text-red-400"
                      >
                        Drain
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {total > pageSize && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 dark:border-gray-700">
            <span className="text-sm text-gray-500 dark:text-gray-400">
              Page {page} of {Math.ceil(total / pageSize)}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page === 1}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded disabled:opacity-50 text-gray-700 dark:text-gray-300"
              >
                Prev
              </button>
              <button
                onClick={() => setPage((p) => p + 1)}
                disabled={page >= Math.ceil(total / pageSize)}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded disabled:opacity-50 text-gray-700 dark:text-gray-300"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </Card>

      <ConfirmDialog
        open={!!drainId}
        title="Drain Node"
        message="This will mark the node offline and stop PostgreSQL. Are you sure?"
        confirmLabel="Drain"
        onConfirm={() => { if (drainId) { drainNode.mutate(drainId); setDrainId(null); }}}
        onCancel={() => setDrainId(null)}
      />
    </div>
  );
}