import { useState } from "react";
import { useNavigate } from "react-router";
import { useCreateCluster } from "~/hooks/useClusters";
import { Card } from "~/components/Card";

export default function CreateClusterPage() {
  const navigate = useNavigate();
  const createCluster = useCreateCluster();
  const [name, setName] = useState("");
  const [engine, setEngine] = useState("POSTGRESQL");
  const [version, setVersion] = useState("16");
  const [replicaCount, setReplicaCount] = useState(1);
  const [replicationMode, setReplicationMode] = useState("ASYNC");
  const [pitrEnabled, setPitrEnabled] = useState(false);
  const [error, setError] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    try {
      await createCluster.mutateAsync({
        name,
        config: {
          engine,
          version,
          replicaCount,
          replicationMode,
          pitrEnabled,
        },
      });
      navigate("/clusters");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create cluster");
    }
  };

  return (
    <div>
      <h2 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">Create Cluster</h2>
      <Card title="Cluster Configuration">
        <form onSubmit={handleSubmit} className="space-y-4 max-w-lg">
          {error && (
            <div className="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
              {error}
            </div>
          )}
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Cluster Name</label>
            <input
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white focus:ring-2 focus:ring-blue-500"
              placeholder="my-cluster"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Engine</label>
              <select
                value={engine}
                onChange={(e) => setEngine(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
              >
                <option value="POSTGRESQL">PostgreSQL</option>
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Version</label>
              <select
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
              >
                <option value="16">16</option>
                <option value="15">15</option>
                <option value="14">14</option>
              </select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Replicas</label>
              <input
                type="number"
                value={replicaCount}
                onChange={(e) => setReplicaCount(Number(e.target.value))}
                min={0}
                max={10}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Replication</label>
              <select
                value={replicationMode}
                onChange={(e) => setReplicationMode(e.target.value)}
                className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
              >
                <option value="ASYNC">Asynchronous</option>
                <option value="SYNC">Synchronous</option>
              </select>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="pitr"
              checked={pitrEnabled}
              onChange={(e) => setPitrEnabled(e.target.checked)}
              className="rounded border-gray-300 dark:border-gray-600"
            />
            <label htmlFor="pitr" className="text-sm text-gray-700 dark:text-gray-300">Enable PITR (Point-in-Time Recovery)</label>
          </div>
          <div className="flex gap-3 pt-2">
            <button
              type="submit"
              disabled={createCluster.isPending}
              className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
            >
              {createCluster.isPending ? "Creating..." : "Create Cluster"}
            </button>
            <button
              type="button"
              onClick={() => navigate("/clusters")}
              className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg"
            >
              Cancel
            </button>
          </div>
        </form>
      </Card>
    </div>
  );
}