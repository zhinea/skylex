import type { ReactNode } from "react";

export function StatCard({ label, value, subtitle }: {
  label: string;
  value: string | number;
  subtitle?: string;
}) {
  return (
    <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <dt className="text-sm font-medium text-gray-500 dark:text-gray-400 truncate">{label}</dt>
      <dd className="mt-1 text-3xl font-semibold text-gray-900 dark:text-white">{value}</dd>
      {subtitle && <dd className="mt-1 text-xs text-gray-500 dark:text-gray-400">{subtitle}</dd>}
    </div>
  );
}