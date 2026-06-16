import type { ReactNode } from "react";

export function Table({ columns, data, emptyText }: {
  columns: { key: string; label: string; render?: (row: Record<string, unknown>) => ReactNode }[];
  data: Record<string, unknown>[];
  emptyText?: string;
}) {
  if (!data.length) {
    return <p className="text-sm text-gray-500 dark:text-gray-400 py-4 text-center">{emptyText || "No data"}</p>;
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-200 dark:border-gray-700">
            {columns.map((col) => (
              <th key={col.key} className="text-left px-4 py-3 font-medium text-gray-600 dark:text-gray-400">
                {col.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row, i) => (
            <tr key={(row.id as string) || i} className="border-b border-gray-100 dark:border-gray-800 hover:bg-gray-50 dark:hover:bg-gray-750">
              {columns.map((col) => (
                <td key={col.key} className="px-4 py-3 text-gray-900 dark:text-white">
                  {col.render ? col.render(row) : String(row[col.key] ?? "")}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}