import { useState } from "react";
import { useAuditLogs } from "~/hooks/useAuditLogs";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";

export default function AuditPage() {
  const [page, setPage] = useState(1);
  const { data, isLoading } = useAuditLogs(page);

  if (isLoading) return <PageSpinner />;

  const entries = data?.entries || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  return (
    <div>
      <h2 className="text-2xl font-bold text-gray-900 dark:text-white mb-6">Audit Logs</h2>

      <Card>
        {entries.length === 0 ? (
          <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">
            No audit log entries yet.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Timestamp</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">User</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Action</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Resource</th>
                  <th className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">Detail</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry, i) => (
                  <tr key={String(entry.id) || String(i)} className="border-b border-gray-100 dark:border-gray-800">
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs whitespace-nowrap">
                      {entry.timestamp ? new Date(entry.timestamp).toLocaleString() : "-"}
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white text-xs">
                      {(entry.userId || "").substring(0, 8) || "-"}
                    </td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{entry.action || "-"}</td>
                    <td className="px-4 py-3 text-gray-900 dark:text-white">{entry.resource || "-"}</td>
                    <td className="px-4 py-3 text-gray-500 dark:text-gray-400 text-xs max-w-xs truncate">
                      {entry.detail || "-"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {total > pageSize && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 dark:border-gray-700">
            <span className="text-sm text-gray-500 dark:text-gray-400">
              Page {page} of {Math.ceil(total / pageSize)}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page === 1}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded disabled:opacity-50 text-gray-700 dark:text-gray-300"
              >
                Prev
              </button>
              <button
                onClick={() => setPage((p) => p + 1)}
                disabled={page >= Math.ceil(total / pageSize)}
                className="px-3 py-1 text-sm border border-gray-300 dark:border-gray-600 rounded disabled:opacity-50 text-gray-700 dark:text-gray-300"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </Card>
    </div>
  );
}