import { useClusters } from "~/hooks/useClusters";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { Link } from "react-router";

export default function DashboardPage() {
  const { data, isLoading } = useClusters(1, 100);

  if (isLoading) return <PageSpinner />;

  const clusters = data?.clusters || [];
  const totalClusters = data?.pagination?.total || clusters.length;
  const healthyClusters = clusters.filter((c) => c.status === "HEALTHY").length;
  const degradedClusters = clusters.filter((c) => c.status === "DEGRADED").length;
  const failedClusters = clusters.filter((c) => c.status === "FAILED").length;

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold text-gray-900 dark:text-white">Dashboard</h2>
        <Link
          to="/clusters/create"
          className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm font-medium rounded-lg transition-colors"
        >
          Create Cluster
        </Link>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <dt className="text-sm font-medium text-gray-500 dark:text-gray-400">Total Clusters</dt>
          <dd className="mt-1 text-3xl font-semibold text-gray-900 dark:text-white">{totalClusters}</dd>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <dt className="text-sm font-medium text-gray-500 dark:text-gray-400">Healthy</dt>
          <dd className="mt-1 text-3xl font-semibold text-green-600">{healthyClusters}</dd>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <dt className="text-sm font-medium text-gray-500 dark:text-gray-400">Degraded</dt>
          <dd className="mt-1 text-3xl font-semibold text-yellow-600">{degradedClusters}</dd>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <dt className="text-sm font-medium text-gray-500 dark:text-gray-400">Failed</dt>
          <dd className="mt-1 text-3xl font-semibold text-red-600">{failedClusters}</dd>
        </div>
      </div>

      <Card title="Recent Clusters">
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
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Status</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Replicas</th>
                </tr>
              </thead>
              <tbody>
                {clusters.slice(0, 10).map((c) => (
                  <tr key={c.id} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-4 py-3">
                      <Link to={`/clusters/${c.id}`} className="text-blue-600 hover:text-blue-800 dark:text-blue-400 font-medium">
                        {c.name}
                      </Link>
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">
                      {c.config?.engine || "POSTGRESQL"}
                    </td>
                    <td className="px-4 py-3">
                      <Badge label={c.status} />
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">
                      {c.config?.replicaCount || 0}
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