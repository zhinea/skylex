import { useState } from "react";
import { useClusters } from "~/hooks/useClusters";
import { useBackups, useCreateBackup, useDeleteBackup } from "~/hooks/useBackups";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { Link } from "react-router";

export default function BackupsPage() {
  const [selectedCluster, setSelectedCluster] = useState("");
  const { data: clustersData } = useClusters(1, 100);
  const { data, isLoading } = useBackups(selectedCluster || undefined);
  const createBackup = useCreateBackup();
  const deleteBackup = useDeleteBackup();
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const clusters = clustersData?.clusters || [];
  const backups = data?.backups || [];

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Backups</h2>
        <div className="flex items-center gap-3">
          <select
            value={selectedCluster}
            onChange={(e) => setSelectedCluster(e.target.value)}
            className="px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
          >
            <option value="">All Clusters</option>
            {clusters.map((c) => (
              <option key={c.id} value={c.id}>{c.name}</option>
            ))}
          </select>
          <Link
            to="/restore"
            className="px-4 py-2 bg-green-600 hover:bg-green-700 text-white text-sm font-medium rounded-lg"
          >
            Restore
          </Link>
        </div>
      </div>

      <Card>
        {isLoading ? (
          <PageSpinner />
        ) : backups.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No backups available. Create a cluster and enable PITR to see backups here.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Cluster</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Type</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Size</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">LSN</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Status</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Created</th>
                  <th className="text-right px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Actions</th>
                </tr>
              </thead>
              <tbody>
                {backups.map((b) => (
                  <tr key={b.id} className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-750">
                    <td className="px-4 py-3 text-gray-900 dark:text-white">
                      {clusters.find((c) => c.id === b.clusterId)?.name || b.clusterId.substring(0, 8)}
                    </td>
                    <td className="px-4 py-3"><Badge label={b.type || "FULL"} /></td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{b.sizeBytes ? `${(Number(b.sizeBytes) / 1024 / 1024).toFixed(1)} MB` : "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs font-mono">{b.lsn || "-"}</td>
                    <td className="px-4 py-3"><Badge label={b.status} /></td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs">{new Date(b.createdAt).toLocaleString()}</td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => setDeleteId(b.id)}
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
      </Card>

      <ConfirmDialog
        open={!!deleteId}
        title="Delete Backup"
        message="Are you sure you want to delete this backup?"
        confirmLabel="Delete"
        onConfirm={() => { if (deleteId) { deleteBackup.mutate(deleteId); setDeleteId(null); }}}
        onCancel={() => setDeleteId(null)}
      />
    </div>
  );
}