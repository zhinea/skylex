import { useState } from "react";
import { useNavigate } from "react-router";
import { useClusters } from "~/hooks/useClusters";
import { useBackups, useCreateRestoreJob } from "~/hooks/useBackups";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";

export default function RestorePage() {
  const navigate = useNavigate();
  const { data: clustersData, isLoading: clustersLoading } = useClusters(1, 100);
  const [selectedCluster, setSelectedCluster] = useState("");
  const { data: backupsData, isLoading: backupsLoading } = useBackups(selectedCluster || undefined);
  const restoreJob = useCreateRestoreJob();
  const [selectedBackup, setSelectedBackup] = useState("");
  const [targetTime, setTargetTime] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  if (clustersLoading) return <PageSpinner />;

  const clusters = clustersData?.clusters || [];
  const backups = backupsData?.backups || [];

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSuccess("");
    try {
      await restoreJob.mutateAsync({
        clusterId: selectedCluster,
        backupId: selectedBackup,
        targetTime: targetTime || undefined,
      });
      setSuccess("Restore job created successfully.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create restore job");
    }
  };

  return (
    <div>
      <h2 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">Restore</h2>
      <div className="max-w-lg">
        <Card title="Point-in-Time Recovery">
          {backups.length === 0 && selectedCluster ? (
            <p className="text-sm text-gray-500 dark:text-gray-400 py-4">
              No backups available for this cluster. Create a backup first.
            </p>
          ) : (
            <form onSubmit={handleSubmit} className="space-y-4">
              {error && (
                <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
                  {error}
                </div>
              )}
              {success && (
                <div className="bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 text-green-700 dark:text-green-400 px-4 py-3 rounded-lg text-sm">
                  {success}
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Source Cluster</label>
                <select
                  value={selectedCluster}
                  onChange={(e) => { setSelectedCluster(e.target.value); setSelectedBackup(""); }}
                  required
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                >
                  <option value="">Select cluster...</option>
                  {clusters.map((c) => (
                    <option key={c.id} value={c.id}>{c.name}</option>
                  ))}
                </select>
              </div>
              {backups.length > 0 && (
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Backup</label>
                  <select
                    value={selectedBackup}
                    onChange={(e) => setSelectedBackup(e.target.value)}
                    required
                    className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                  >
                    <option value="">Select backup...</option>
                    {backups.filter((b) => b.status === "COMPLETED").map((b) => (
                      <option key={b.id} value={b.id}>
                        {new Date(b.createdAt).toLocaleString()} - {b.type || "FULL"} ({b.lsn || "N/A"})
                      </option>
                    ))}
                  </select>
                </div>
              )}
              <div>
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Target Time (optional, for PITR)
                </label>
                <input
                  type="datetime-local"
                  value={targetTime}
                  onChange={(e) => setTargetTime(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                />
              </div>
              <div className="flex gap-3">
                <button
                  type="submit"
                  disabled={restoreJob.isPending || !selectedCluster || !selectedBackup}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
                >
                  {restoreJob.isPending ? "Creating..." : "Start Restore"}
                </button>
                <button
                  type="button"
                  onClick={() => navigate("/backups")}
                  className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg"
                >
                  Back to Backups
                </button>
              </div>
            </form>
          )}
        </Card>
      </div>
    </div>
  );
}