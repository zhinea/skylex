import { useState } from "react";
import { useNodes, useDrainNode, useRejoinNode } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { InstallAgentModal } from "~/components/InstallAgentModal";
import { AgentStatus } from "~/components/AgentStatus";

export default function NodesPage() {
  const [page, setPage] = useState(1);
  const [installOpen, setInstallOpen] = useState(false);
  const { data, isLoading } = useNodes(undefined, page);
  const drainNode = useDrainNode();
  const rejoinNode = useRejoinNode();
  const [drainId, setDrainId] = useState<string | null>(null);
  const [rejoinId, setRejoinId] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const nodes = data?.nodes || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Nodes</h2>
        <button
          onClick={() => setInstallOpen(true)}
          className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
        >
          Add Node
        </button>
      </div>

      <Card>
        {nodes.length === 0 ? (
          <div className="py-10 text-center">
            <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-2">
              No agents yet
            </h3>
            <p className="text-sm text-gray-500 dark:text-gray-400 mb-6 max-w-md mx-auto">
              Add your first database server by installing the Skylex agent. You&apos;ll get a copy-paste command to run on the target host.
            </p>
            <button
              onClick={() => setInstallOpen(true)}
              className="px-4 py-2 text-sm font-medium bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors"
            >
              Install Agent
            </button>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Hostname</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Cluster</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Role</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Address</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Agent</th>
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
                    <td className="px-4 py-3">
                      <AgentStatus connected={n.agentConnected} latencyMs={n.agentLatencyMs} />
                    </td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{n.agentVersion || "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">
                      {n.lastSeen ? new Date(n.lastSeen).toLocaleString() : "-"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex items-center justify-end gap-2">
                        {n.clusterId && n.statusDetail === "stopped" && (
                          <button
                            onClick={() => setRejoinId(n.id)}
                            className="text-xs text-purple-600 hover:text-purple-800 dark:text-purple-400"
                            title="Rejoin cluster"
                          >
                            Rejoin
                          </button>
                        )}
                        <button
                          onClick={() => setDrainId(n.id)}
                          className="text-xs text-red-600 hover:text-red-800 dark:text-red-400"
                        >
                          Drain
                        </button>
                      </div>
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

      <ConfirmDialog
        open={!!rejoinId}
        title="Rejoin Node"
        message="This will repoint the node to follow the current primary and restart. Any divergent data will be overwritten. Are you sure?"
        confirmLabel="Rejoin"
        onConfirm={() => { if (rejoinId) { rejoinNode.mutate(rejoinId); setRejoinId(null); }}}
        onCancel={() => setRejoinId(null)}
      />

      <InstallAgentModal open={installOpen} onClose={() => setInstallOpen(false)} />
    </div>
  );
}
