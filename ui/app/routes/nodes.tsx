import { useState } from "react";
import { useNodes, useDrainNode } from "~/hooks/useNodes";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { InstallAgentModal } from "~/components/InstallAgentModal";

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
        PG not installed
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200">
        {version || "PG installed"}
      </span>
      <span
        className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
          dataInitialized
            ? "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200"
            : "bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-400"
        }`}
      >
        {dataInitialized ? "data dir ready" : "not initialized"}
      </span>
    </span>
  );
}

export default function NodesPage() {
  const [page, setPage] = useState(1);
  const [installOpen, setInstallOpen] = useState(false);
  const { data, isLoading } = useNodes(undefined, page);
  const drainNode = useDrainNode();
  const [drainId, setDrainId] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const nodes = data?.nodes || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  const missingPgCount = nodes.filter((n) => !n.postgresInstalled).length;

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

      {missingPgCount > 0 && nodes.length > 0 && (
        <div className="mb-4 flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-700">
          <span className="text-yellow-600 dark:text-yellow-400 mt-0.5">⚠</span>
          <p className="text-sm text-yellow-800 dark:text-yellow-200">
            {missingPgCount === 1
              ? "1 node does not have PostgreSQL installed."
              : `${missingPgCount} nodes do not have PostgreSQL installed.`}{" "}
            Install PostgreSQL on those hosts before adding them to a cluster.
          </p>
        </div>
      )}

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
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">PostgreSQL</th>
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
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </td>
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

      <InstallAgentModal open={installOpen} onClose={() => setInstallOpen(false)} />
    </div>
  );
}