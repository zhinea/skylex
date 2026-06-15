import type { Route } from "./+types/restore";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Restore - Skylex" }];
}

export default function Restore() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Restore
      </h1>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          Select a backup to restore. Point-in-Time Recovery (PITR) will be available
          once WAL archiving is configured.
        </p>
      </div>
    </div>
  );
}