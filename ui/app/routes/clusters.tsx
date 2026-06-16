import { useState } from "react";
import { Link } from "react-router";
import { useClusters, useDeleteCluster, useFailoverCluster } from "~/hooks/useClusters";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";

export default function ClustersPage() {
  const [page, setPage] = useState(1);
  const { data, isLoading } = useClusters(page);
  const deleteCluster = useDeleteCluster();
  const failoverCluster = useFailoverCluster();
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [failoverId, setFailoverId] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const clusters = data?.clusters || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 20;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Clusters</h2>
        <Link
          to="/clusters/create"
          className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium rounded-lg transition-colors"
        >
          Create Cluster
        </Link>
      </div>

      <Card>
        {clusters.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No clusters yet. Create your first cluster to get started.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Name</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Engine</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Version</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Replicas</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Status</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {clusters.map((c) => (
                  <tr key={c.id} className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-750">
                    <td className="px-4 py-3">
                      <Link to={`/clusters/${c.id}`} className="text-blue-600 hover:text-blue-800 dark:text-blue-400 font-medium">
                        {c.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{c.config?.engine || "POSTGRESQL"}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{c.config?.version || "16"}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{c.config?.replicaCount || 0}</td>
                    <td className="px-4 py-3"><Badge label={c.status} /></td>
                    <td className="px-4 py-3 text-right space-x-2">
                      <button
                        onClick={() => setFailoverId(c.id)}
                        className="text-xs text-yellow-600 hover:text-yellow-800 dark:text-yellow-400"
                        title="Failover"
                      >
                        Failover
                      </button>
                      <button
                        onClick={() => setDeleteId(c.id)}
                        className="text-xs text-red-600 hover:text-red-800 dark:text-red-400"
                      >
                        Delete
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
        open={!!deleteId}
        title="Delete Cluster"
        message="Are you sure you want to delete this cluster? This action cannot be undone."
        confirmLabel="Delete"
        onConfirm={() => { if (deleteId) { deleteCluster.mutate(deleteId); setDeleteId(null); }}}
        onCancel={() => setDeleteId(null)}
      />

      <ConfirmDialog
        open={!!failoverId}
        title="Failover Cluster"
        message="This will trigger a manual failover. The current replica will be promoted to primary."
        confirmLabel="Failover"
        onConfirm={() => { if (failoverId) { failoverCluster.mutate(failoverId); setFailoverId(null); }}}
        onCancel={() => setFailoverId(null)}
      />
    </div>
  );
}