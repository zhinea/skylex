import type { Route } from "./+types/settings";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Settings - Skylex" }];
}

export default function Settings() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Settings
      </h1>
      <div className="space-y-4">
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Users</h2>
          <p className="text-gray-500 dark:text-gray-400 text-sm">
            Manage users and roles. RBAC coming in Phase 4.
          </p>
        </div>
        <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Agent Tokens</h2>
          <p className="text-gray-500 dark:text-gray-400 text-sm">
            Generate and manage agent registration tokens.
          </p>
        </div>
      </div>
    </div>
  );
}