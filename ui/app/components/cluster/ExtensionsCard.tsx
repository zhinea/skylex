import { useState } from "react";
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

// displayStatus reconciles the applied status with the desired toggle so the
// badge never contradicts the switch. When the desired state differs from what
// is actually applied on the cluster, the extension is shown as 'pending'.
// 'failed' always wins so apply errors stay visible.
function displayStatus(desired: boolean, status: string): string {
  if (status === "failed") return "failed";
  if (desired) return status === "ready" ? "ready" : "pending";
  // desired off
  return status === "ready" ? "pending" : "off";
}

export function ExtensionsCard({ clusterId }: ExtensionsCardProps) {
  const { data, isLoading } = useExtensions(clusterId);
  const setExtension = useSetExtension(clusterId);
  const applyExtensions = useApplyExtensions(clusterId);

  // Local desired-state overrides. Toggling only mutates this map; nothing is
  // sent to the server until Apply. Keyed by extension name -> desired enabled.
  const [pending, setPending] = useState<Record<string, boolean>>({});
  const [applyError, setApplyError] = useState<string | null>(null);

  const extensions = data?.extensions ?? [];
  // Effective desired state = local override if present, else server's stored value.
  const desiredOf = (ext: Extension) => pending[ext.name] ?? ext.enabled;

  // A change is pending when a local override differs from the stored value.
  const hasPendingChanges = extensions.some(
    (e) => e.name in pending && pending[e.name] !== e.enabled,
  );

  function toggle(ext: Extension) {
    setPending((prev) => {
      const next = { ...prev };
      const desired = !desiredOf(ext);
      if (desired === ext.enabled) {
        // Back to the stored value — drop the override so it's no longer pending.
        delete next[ext.name];
      } else {
        next[ext.name] = desired;
      }
      return next;
    });
  }

  async function apply() {
    setApplyError(null);
    const changed = extensions.filter((e) => e.name in pending && pending[e.name] !== e.enabled);
    try {
      // Persist each changed toggle, then converge the cluster. The catalog is a
      // small fixed set, so this bounded sequential flush is acceptable; the
      // server serializes them on the per-cluster lock.
      for (const e of changed) {
        await setExtension.mutateAsync({ name: e.name, enabled: pending[e.name] });
      }
      await applyExtensions.mutateAsync();
      setPending({});
    } catch (err) {
      setApplyError(err instanceof Error ? err.message : "Failed to apply extensions");
    }
  }

  const isApplying = setExtension.isPending || applyExtensions.isPending;

  if (isLoading) return <PageSpinner />;

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <Puzzle className="h-4 w-4" /> Extensions
        </CardTitle>
        <Button
          size="sm"
          onClick={apply}
          disabled={isApplying || !hasPendingChanges}
        >
          {isApplying ? "Applying…" : "Apply"}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <FeatureNote title="How extensions work">
          Toggle the extensions you want, then click Apply. Skylex runs
          CREATE/DROP EXTENSION on the primary across your managed databases — no
          restart and no downtime. Changes replicate to standbys automatically.
        </FeatureNote>

        {applyError && (
          <p className="text-xs text-red-500">{applyError}</p>
        )}

        <ul className="divide-y divide-border rounded-md border border-border">
          {extensions.map((ext) => {
            const desired = desiredOf(ext);
            return (
              <li key={ext.name} className="flex items-center justify-between gap-4 px-4 py-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm text-foreground">{ext.label}</span>
                    {statusBadge(displayStatus(desired, ext.status))}
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
                    checked={desired}
                    onChange={() => toggle(ext)}
                    disabled={isApplying}
                    aria-label={`Toggle ${ext.label}`}
                  />
                  <div className="h-5 w-9 rounded-full bg-muted transition-colors peer-checked:bg-emerald-500 peer-focus-visible:ring-2 peer-focus-visible:ring-ring after:absolute after:left-0.5 after:top-0.5 after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-4" />
                </label>
              </li>
            );
          })}
        </ul>
      </CardContent>
    </Card>
  );
}
