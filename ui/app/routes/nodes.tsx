import type { Route } from "./+types/nodes";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Nodes - Skylex" }];
}

export default function Nodes() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Nodes
      </h1>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          No nodes registered. Deploy agents to manage your database servers.
        </p>
      </div>
    </div>
  );
}