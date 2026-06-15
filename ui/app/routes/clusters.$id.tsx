import type { Route } from "./+types/clusters-detail";
import { Link, useParams } from "react-router";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Cluster Detail - Skylex" }];
}

export default function ClusterDetail() {
  const { id } = useParams();

  return (
    <div>
      <div className="flex items-center gap-4 mb-6">
        <Link to="/clusters" className="text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200">
          &larr; Back
        </Link>
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">
          Cluster {id}
        </h1>
      </div>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          Cluster detail view will be available in the next phase.
        </p>
      </div>
    </div>
  );
}