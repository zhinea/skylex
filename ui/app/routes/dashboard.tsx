import type { Route } from "./+types/dashboard";
import { Link } from "react-router";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Dashboard - Skylex" }];
}

export default function Dashboard() {
  const stats = [
    { label: "Clusters", value: "0", to: "/clusters", color: "bg-indigo-500" },
    { label: "Nodes", value: "0", to: "/nodes", color: "bg-emerald-500" },
    { label: "Backups", value: "0", to: "/backups", color: "bg-amber-500" },
    { label: "Alerts", value: "0", to: "/audit", color: "bg-red-500" },
  ];

  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Dashboard
      </h1>
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        {stats.map((stat) => (
          <Link
            key={stat.label}
            to={stat.to}
            className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6 hover:shadow-md transition-shadow"
          >
            <div className="flex items-center gap-4">
              <div className={`w-3 h-3 rounded-full ${stat.color}`} />
              <div>
                <div className="text-3xl font-bold text-gray-900 dark:text-white">
                  {stat.value}
                </div>
                <div className="text-sm text-gray-500 dark:text-gray-400">
                  {stat.label}
                </div>
              </div>
            </div>
          </Link>
        ))}
      </div>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
          Recent Events
        </h2>
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          No recent events. Get started by creating a cluster.
        </p>
        <Link
          to="/clusters"
          className="inline-block mt-4 px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors"
        >
          Create Cluster
        </Link>
      </div>
    </div>
  );
}