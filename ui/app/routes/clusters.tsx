import type { Route } from "./+types/clusters";
import { Link } from "react-router";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Clusters - Skylex" }];
}

export default function Clusters() {
  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
          Clusters
        </h1>
        <Link
          to="/clusters/create"
          className="px-4 py-2 bg-indigo-600 text-white rounded-lg text-sm font-medium hover:bg-indigo-700 transition-colors"
        >
          Create Cluster
        </Link>
      </div>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          No clusters yet. Create your first PostgreSQL cluster to get started.
        </p>
      </div>
    </div>
  );
}