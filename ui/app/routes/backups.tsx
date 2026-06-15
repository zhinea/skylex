import type { Route } from "./+types/backups";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Backups - Skylex" }];
}

export default function Backups() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Backups
      </h1>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          No backups available. Create a cluster and enable PITR to see backups here.
        </p>
      </div>
    </div>
  );
}