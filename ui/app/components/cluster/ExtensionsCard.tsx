import { useExtensions, useSetExtension, useApplyExtensions, type Extension } from "~/hooks/useExtensions";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import { Puzzle } from "lucide-react";
import { FeatureNote } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";

interface ExtensionsCardProps {
  clusterId: string;
}

function statusBadge(status: string) {
  const map: Record<string, string> = {
    ready: "bg-emerald-500/15 text-emerald-500",
    pending: "bg-amber-500/15 text-amber-500",
    failed: "bg-red-500/15 text-red-500",
    off: "bg-muted text-muted-foreground",
  };
  const cls = map[status] ?? map.off;
  return (
    <span className={`inline-flex items-center rounded px-2 py-0.5 text-xs font-medium ${cls}`}>
      {status}
    </span>
  );
}

export function ExtensionsCard({ clusterId }: ExtensionsCardProps) {
  const { data, isLoading } = useExtensions(clusterId);
  const setExtension = useSetExtension(clusterId);
  const applyExtensions = useApplyExtensions(clusterId);

  const extensions = data?.extensions ?? [];
  // A change is pending apply when desired (enabled) and applied (status) disagree.
  const hasPendingChanges = extensions.some(
    (e) => (e.enabled && e.status !== "ready") || (!e.enabled && e.status === "ready"),
  );

  function toggle(ext: Extension) {
    setExtension.mutate({ name: ext.name, enabled: !ext.enabled });
  }

  if (isLoading) return <PageSpinner />;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Puzzle className="h-4 w-4" /> Extensions
        </CardTitle>
        <Button
          size="sm"
          onClick={() => applyExtensions.mutate()}
          disabled={applyExtensions.isPending || !hasPendingChanges}
        >
          {applyExtensions.isPending ? "Applying…" : "Apply"}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <FeatureNote title="How extensions work">
          Toggle the extensions you want, then click Apply. Skylex runs
          CREATE/DROP EXTENSION on the primary across your managed databases — no
          restart and no downtime. Changes replicate to standbys automatically.
        </FeatureNote>

        {applyExtensions.isError && (
          <p className="text-xs text-red-500">
            {applyExtensions.error instanceof Error ? applyExtensions.error.message : "Failed to apply extensions"}
          </p>
        )}

        <ul className="divide-y divide-border rounded-md border border-border">
          {extensions.map((ext) => (
            <li key={ext.name} className="flex items-center justify-between gap-4 px-4 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm text-foreground">{ext.label}</span>
                  {statusBadge(ext.status)}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">{ext.description}</p>
                {ext.status === "failed" && ext.error && (
                  <p className="mt-1 text-xs text-red-500">{ext.error}</p>
                )}
              </div>
              <label className="relative inline-flex shrink-0 cursor-pointer items-center">
                <input
                  type="checkbox"
                  className="peer sr-only"
                  checked={ext.enabled}
                  onChange={() => toggle(ext)}
                  disabled={setExtension.isPending}
                  aria-label={`Toggle ${ext.label}`}
                />
                <div className="h-5 w-9 rounded-full bg-muted transition-colors peer-checked:bg-emerald-500 peer-focus-visible:ring-2 peer-focus-visible:ring-ring after:absolute after:left-0.5 after:top-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4" />
              </label>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
