const statusColors: Record<string, string> = {
  HEALTHY: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400",
  CREATING: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400",
  DEGRADED: "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400",
  FAILED: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
  DELETING: "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400",
  RUNNING: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400",
  COMPLETED: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400",
  PRIMARY: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400",
  REPLICA: "bg-gray-100 text-gray-800 dark:bg-gray-900/30 dark:text-gray-400",
  online: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400",
  offline: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
  drained: "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-400",
  admin: "bg-purple-100 text-purple-800 dark:bg-purple-900/30 dark:text-purple-400",
  operator: "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400",
  viewer: "bg-gray-100 text-gray-600 dark:bg-gray-900/30 dark:text-gray-400",
  Connected: "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400",
  Disconnected: "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400",
};

export function Badge({ label, className = "" }: { label: string; className?: string }) {
  const color = statusColors[label] || statusColors[label.toLowerCase()] || "bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400";
  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${color} ${className}`}>
      {label}
    </span>
  );
}
