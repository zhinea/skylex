import type { Route } from "./+types/storage";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Storage - Skylex" }];
}

export default function Storage() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Storage Configurations
      </h1>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          No storage configurations. Add an S3-compatible storage backend to enable backups.
        </p>
      </div>
    </div>
  );
}