import type { Route } from "./+types/audit";

export function meta({}: Route.MetaArgs) {
  return [{ title: "Audit Logs - Skylex" }];
}

export default function AuditLogs() {
  return (
    <div>
      <h1 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">
        Audit Logs
      </h1>
      <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <p className="text-gray-500 dark:text-gray-400 text-sm">
          Audit logs will be available in the next phase.
        </p>
      </div>
    </div>
  );
}