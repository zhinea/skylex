import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
import { useCreateCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { Card } from "~/components/Card";
import { Badge } from "~/components/Badge";
import { PageSpinner } from "~/components/Spinner";

const STEPS = ["Verify Nodes", "Configure", "Review"] as const;

export default function CreateClusterPage() {
  const navigate = useNavigate();
  const createCluster = useCreateCluster();
  const { data: nodesData } = useNodes();

  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [engine, setEngine] = useState("POSTGRESQL");
  const [version, setVersion] = useState("16");
  const [replicaCount, setReplicaCount] = useState(0);
  const [replicationMode, setReplicationMode] = useState("ASYNC");
  const [pitrEnabled, setPitrEnabled] = useState(false);
  const [error, setError] = useState("");

  const idleNodes = useMemo(
    () => (nodesData?.nodes || []).filter((n) => !n.clusterId),
    [nodesData],
  );

  const neededNodes = replicaCount + 1;
  const pgReadyNodes = useMemo(
    () => idleNodes.filter((n) => n.postgresInstalled),
    [idleNodes],
  );
  const missingPgNodes = useMemo(
    () => idleNodes.filter((n) => !n.postgresInstalled),
    [idleNodes],
  );

  const canProceedFromStep0 = idleNodes.length >= neededNodes && missingPgNodes.length === 0;
  const canProceedFromStep1 = name.trim().length > 0;

  const handleSubmit = async () => {
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

      {/* Step indicator */}
      <div className="flex items-center gap-2 mb-8">
        {STEPS.map((label, i) => (
          <div key={label} className="flex items-center gap-2">
            <button
              onClick={() => i < step ? setStep(i) : undefined}
              disabled={i > step}
              className={`flex items-center justify-center w-8 h-8 rounded-full text-sm font-semibold transition-colors
                ${i < step ? "bg-green-600 text-white cursor-pointer" : ""}
                ${i === step ? "bg-blue-600 text-white" : ""}
                ${i > step ? "bg-gray-200 dark:bg-gray-700 text-gray-500 dark:text-gray-400" : ""}
              `}
            >
              {i < step ? "✓" : i + 1}
            </button>
            <span
              className={`text-sm font-medium ${
                i <= step
                  ? "text-gray-900 dark:text-white"
                  : "text-gray-400 dark:text-gray-500"
              }`}
            >
              {label}
            </span>
            {i < STEPS.length - 1 && (
              <span className="text-gray-300 dark:text-gray-600 mx-1">→</span>
            )}
          </div>
        ))}
      </div>

      {error && (
        <div className="mb-6 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 px-4 py-3 rounded-lg text-sm">
          {error}
        </div>
      )}

      {/* Step 0: Verify Nodes */}
      {step === 0 && (
        <Card title="Step 1: Verify Available Nodes">
          {!nodesData ? (
            <PageSpinner />
          ) : (
            <div className="space-y-4">
              <p className="text-sm text-gray-500 dark:text-gray-400">
                Your cluster needs {neededNodes} node{neededNodes !== 1 ? "s" : ""} (1 primary + {replicaCount} replica{replicaCount !== 1 ? "s" : ""}).
                Each node must have PostgreSQL installed.
              </p>

              {/* Idle nodes summary */}
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4 text-center">
                  <div className="text-2xl font-bold text-gray-900 dark:text-white">{idleNodes.length}</div>
                  <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">Idle Nodes Available</div>
                </div>
                <div className="bg-green-50 dark:bg-green-900/20 rounded-lg p-4 text-center">
                  <div className="text-2xl font-bold text-green-700 dark:text-green-400">{pgReadyNodes.length}</div>
                  <div className="text-xs text-green-600 dark:text-green-400 mt-1">PG Ready</div>
                </div>
                <div className={`rounded-lg p-4 text-center ${missingPgNodes.length > 0 ? "bg-yellow-50 dark:bg-yellow-900/20" : "bg-gray-50 dark:bg-gray-700/50"}`}>
                  <div className={`text-2xl font-bold ${missingPgNodes.length > 0 ? "text-yellow-700 dark:text-yellow-400" : "text-gray-900 dark:text-white"}`}>
                    {missingPgNodes.length}
                  </div>
                  <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">Missing PostgreSQL</div>
                </div>
              </div>

              {/* Validation warnings */}
              {idleNodes.length < neededNodes && (
                <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-red-50 border border-red-200 dark:bg-red-900/20 dark:border-red-700">
                  <span className="text-red-600 dark:text-red-400 mt-0.5">✗</span>
                  <p className="text-sm text-red-800 dark:text-red-200">
                    Not enough idle nodes. You need {neededNodes} but only {idleNodes.length} are available.
                    Decrease replica count or register more agents.
                  </p>
                </div>
              )}

              {missingPgNodes.length > 0 && (
                <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 dark:bg-yellow-900/20 dark:border-yellow-700">
                  <span className="text-yellow-600 dark:text-yellow-400 mt-0.5">⚠</span>
                  <div className="text-sm text-yellow-800 dark:text-yellow-200">
                    <p className="font-medium mb-1">PostgreSQL not installed on:</p>
                    <ul className="list-disc list-inside space-y-0.5">
                      {missingPgNodes.map((n) => (
                        <li key={n.id}>{n.hostname}</li>
                      ))}
                    </ul>
                    <p className="mt-2">
                      Install PostgreSQL on these hosts before creating a cluster.
                    </p>
                  </div>
                </div>
              )}

              {canProceedFromStep0 && (
                <div className="flex items-start gap-3 px-4 py-3 rounded-lg bg-green-50 border border-green-200 dark:bg-green-900/20 dark:border-green-700">
                  <span className="text-green-600 dark:text-green-400 mt-0.5">✓</span>
                  <p className="text-sm text-green-800 dark:text-green-200">
                    All {neededNodes} required node{neededNodes !== 1 ? "s" : ""} are available with PostgreSQL installed.
                  </p>
                </div>
              )}

              {/* Idle nodes table */}
              {idleNodes.length > 0 && (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-gray-200 dark:border-gray-700">
                        <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Hostname</th>
                        <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Address</th>
                        <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">PostgreSQL</th>
                        <th className="text-left px-3 py-2 font-medium text-gray-600 dark:text-gray-400">Status</th>
                      </tr>
                    </thead>
                    <tbody>
                      {idleNodes.map((n) => (
                        <tr key={n.id} className="border-b border-gray-100 dark:border-gray-800">
                          <td className="px-3 py-2 text-gray-900 dark:text-white font-medium">{n.hostname}</td>
                          <td className="px-3 py-2 text-gray-500 dark:text-gray-400">{n.address}:{n.port}</td>
                          <td className="px-3 py-2">
                            {n.postgresInstalled ? (
                              <span className="text-xs text-green-600 dark:text-green-400">{n.postgresVersion || "installed"}</span>
                            ) : (
                              <Badge label="not installed" />
                            )}
                          </td>
                          <td className="px-3 py-2">
                            {n.statusDetail ? (
                              <span className="text-xs text-gray-500 dark:text-gray-400">{n.statusDetail}</span>
                            ) : (
                              <Badge label={n.clusterId ? "assigned" : "idle"} />
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}

              <div className="flex gap-3 pt-2">
                <button
                  onClick={() => setStep(1)}
                  disabled={!canProceedFromStep0}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
                >
                  Next: Configure
                </button>
                <button
                  type="button"
                  onClick={() => navigate("/clusters")}
                  className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </Card>
      )}

      {/* Step 1: Configure */}
      {step === 1 && (
        <Card title="Step 2: Cluster Configuration">
          <div className="space-y-4 max-w-lg">
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
                <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">
                  Replicas ({idleNodes.length - 1} available)
                </label>
                <input
                  type="number"
                  value={replicaCount}
                  onChange={(e) => setReplicaCount(Number(e.target.value))}
                  min={0}
                  max={Math.max(0, idleNodes.length - 1)}
                  className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg text-sm bg-white dark:bg-gray-700 text-gray-900 dark:text-white"
                />
                {replicaCount > idleNodes.length - 1 && (
                  <p className="text-xs text-red-600 dark:text-red-400 mt-1">
                    Only {idleNodes.length} idle nodes available. Max replicas: {Math.max(0, idleNodes.length - 1)}.
                  </p>
                )}
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

            {replicaCount > 0 && !pgReadyNodes.length && (
              <p className="text-xs text-yellow-600 dark:text-yellow-400">
                No idle nodes have PostgreSQL installed. You may need to install it before replicas can be provisioned.
              </p>
            )}

            <div className="flex gap-3 pt-2">
              <button
                onClick={() => setStep(0)}
                className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg"
              >
                Back
              </button>
              <button
                onClick={() => setStep(2)}
                disabled={!canProceedFromStep1}
                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
              >
                Next: Review
              </button>
            </div>
          </div>
        </Card>
      )}

      {/* Step 2: Review */}
      {step === 2 && (
        <Card title="Step 3: Review & Create">
          <div className="space-y-4 max-w-lg">
            <dl className="space-y-2 text-sm">
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">Cluster Name</dt>
                <dd className="text-gray-900 dark:text-white font-medium">{name}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">Engine</dt>
                <dd className="text-gray-900 dark:text-white">{engine}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">Version</dt>
                <dd className="text-gray-900 dark:text-white">{version}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">Nodes</dt>
                <dd className="text-gray-900 dark:text-white">
                  1 primary + {replicaCount} replica{replicaCount !== 1 ? "s" : ""} ({neededNodes} total)
                </dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">Replication</dt>
                <dd className="text-gray-900 dark:text-white">
                  {replicationMode === "SYNC" ? "Synchronous" : "Asynchronous"}
                </dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">PITR</dt>
                <dd className="text-gray-900 dark:text-white">{pitrEnabled ? "Enabled" : "Disabled"}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-gray-500 dark:text-gray-400">Idle Nodes Available</dt>
                <dd className="text-gray-900 dark:text-white">{idleNodes.length}</dd>
              </div>
            </dl>

            <div className="bg-gray-50 dark:bg-gray-700/50 rounded-lg p-4">
              <p className="text-sm text-gray-600 dark:text-gray-400">
                After creation, the server will queue initialization commands for the primary node
                (pg_init, pg_start, pg_create_repl_user) and any replica nodes.
              </p>
            </div>

            <div className="flex gap-3 pt-2">
              <button
                onClick={() => setStep(1)}
                className="px-4 py-2 bg-gray-100 hover:bg-gray-200 dark:bg-gray-700 dark:hover:bg-gray-600 text-gray-700 dark:text-gray-300 text-sm font-medium rounded-lg"
              >
                Back
              </button>
              <button
                onClick={handleSubmit}
                disabled={createCluster.isPending}
                className="px-4 py-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
              >
                {createCluster.isPending ? "Creating..." : "Create Cluster"}
              </button>
            </div>
          </div>
        </Card>
      )}
    </div>
  );
}