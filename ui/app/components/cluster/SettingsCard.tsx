import { useState, useMemo, useEffect } from "react";
import type { Cluster } from "~/hooks/useClusters";
import { useClusterSettings, useUpdateClusterSettings } from "~/hooks/useClusterSettings";
import { useToast } from "~/components/ui/toast";
import { Button } from "~/components/ui/button";
import { SettingInput, curatedSettings, validateSettingValue } from "~/components/SettingInput";
import { FeatureNote } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";

interface SettingsCardProps {
  clusterId: string;
  cluster: Cluster;
}

type Submenu = "general" | "memory" | "connectivity";

export function SettingsCard({ clusterId, cluster }: SettingsCardProps) {
  const { data, isLoading } = useClusterSettings(clusterId);
  const update = useUpdateClusterSettings();
  const toast = useToast();
  const [values, setValues] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});
  const [saved, setSaved] = useState(false);
  const [activeSubmenu, setActiveSubmenu] = useState<Submenu>("general");

  const parameters = useMemo(() => data?.settings?.parameters ?? {}, [data?.settings?.parameters]);

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const s of curatedSettings) {
      next[s.key] = parameters[s.key] ?? "";
    }
    setValues(next);
  }, [parameters]);

  const memorySettingsKeys = ["shared_buffers", "work_mem"];
  const connectivitySettingsKeys = ["max_connections", "wal_level", "max_wal_senders"];

  const memorySettings = curatedSettings.filter((s) => memorySettingsKeys.includes(s.key));
  const connectivitySettings = curatedSettings.filter((s) => connectivitySettingsKeys.includes(s.key));

  const dirty = useMemo(() => {
    let changed = false;
    for (const s of curatedSettings) {
      if ((values[s.key] ?? "") !== (parameters[s.key] ?? "")) {
        changed = true;
      }
    }
    return changed;
  }, [values, parameters]);

  function handleChange(key: string, value: string) {
    setValues((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => ({ ...prev, [key]: validateSettingValue(key, value) }));
    setSaved(false);
  }

  function handleSave() {
    const nextErrors: Record<string, string | null> = {};
    const payload: Record<string, string> = {};

    for (const s of curatedSettings) {
      const v = values[s.key]?.trim() ?? "";
      const err = validateSettingValue(s.key, v);
      nextErrors[s.key] = err;
      if (!err && v) {
        payload[s.key] = v;
      }
    }

    setErrors(nextErrors);
    if (Object.values(nextErrors).some(Boolean)) {
      return;
    }

    update.mutate(
      { clusterId, settings: payload },
      {
        onSuccess: () => {
          setSaved(true);
          toast.success("Settings applied successfully", "Configuration changes have been queued for all nodes.");
          setTimeout(() => setSaved(false), 3000);
        },
        onError: (err) => {
          toast.error("Failed to update settings", err instanceof Error ? err.message : "An error occurred");
        },
      },
    );
  }

  const submenuItems = [
    { id: "general", label: "General Info", icon: "info" },
    { id: "memory", label: "Resource Tuning", icon: "memory" },
    { id: "connectivity", label: "Connectivity & WAL", icon: "hub" },
  ] as const;

  return (
    <div className="flex flex-col md:flex-row gap-6 items-start">
      {/* Settings Child Sidebar */}
      <aside className="w-full md:w-56 shrink-0 flex flex-row md:flex-col gap-0.5 overflow-x-auto md:overflow-x-visible pb-2 md:pb-0 md:pr-2">
        {submenuItems.map((item) => {
          const isActive = activeSubmenu === item.id;
          return (
            <button
              key={item.id}
              onClick={() => setActiveSubmenu(item.id)}
              className={`flex items-center gap-2 px-3 py-2 rounded text-xs transition-colors cursor-pointer text-left whitespace-nowrap md:w-full ${
                isActive
                  ? "bg-neutral-100 dark:bg-neutral-900 text-foreground font-medium"
                  : "text-muted-foreground hover:bg-neutral-50 dark:hover:bg-neutral-950 hover:text-foreground"
              }`}
            >
              <span className="material-symbols-outlined text-lg">{item.icon}</span>
              <span>{item.label}</span>
            </button>
          );
        })}
      </aside>

      {/* Settings Content Area */}
      <div className="flex-1 w-full space-y-6">
        {activeSubmenu === "general" && (
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            {/* Configuration Card */}
            <div className="v-card rounded-lg overflow-hidden">
              <div className="px-4 py-3 border-b border-border flex items-center gap-2 text-foreground">
                <span className="material-symbols-outlined text-lg text-foreground">settings</span>
                <h3 className="text-xs font-semibold">Configuration</h3>
              </div>
              <div className="p-4">
                <dl className="space-y-2 text-xs">
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Engine</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.engine || "POSTGRESQL"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Version</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.version || "16"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Service Location</dt>
                    <dd className="text-foreground font-semibold">
                      {cluster.serviceLocation === "SERVICE_LOCATION_DOCKER" || cluster.config?.serviceLocation === "SERVICE_LOCATION_DOCKER"
                        ? "Dockerized"
                        : "Native"}
                    </dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Replication</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.replicationMode || "ASYNC"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Replicas</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.replicaCount || 0}</dd>
                  </div>
                  <div className="flex justify-between py-1.5 border-b border-border/40">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">PITR</dt>
                    <dd className="text-foreground font-semibold">{cluster.config?.pitrEnabled ? "Enabled" : "Disabled"}</dd>
                  </div>
                  <div className="flex justify-between py-1.5">
                    <dt className="text-muted-foreground font-medium uppercase tracking-wider text-[10px]">Created</dt>
                    <dd className="text-foreground font-semibold">{new Date(cluster.createdAt).toLocaleString()}</dd>
                  </div>
                </dl>
              </div>
            </div>

            {/* Labels Card */}
            <div className="v-card rounded-lg overflow-hidden">
              <div className="px-4 py-3 border-b border-border flex items-center gap-2 text-foreground">
                <span className="material-symbols-outlined text-lg text-foreground">layers</span>
                <h3 className="text-xs font-semibold">Labels</h3>
              </div>
              <div className="p-4">
                {cluster.config?.labels && Object.keys(cluster.config.labels).length > 0 ? (
                  <div className="flex flex-wrap gap-2">
                    {Object.entries(cluster.config.labels).map(([k, v]) => (
                      <span key={k} className="px-2.5 py-1 bg-muted/40 text-foreground border border-border rounded-md text-xs font-mono">
                        {k}: {v}
                      </span>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">No labels configured</p>
                )}
              </div>
            </div>
          </div>
        )}

        {activeSubmenu === "memory" && (
          <div className="v-card rounded-lg overflow-hidden">
            <div className="px-4 py-3 border-b border-border flex items-center gap-2 text-foreground">
              <span className="material-symbols-outlined text-lg text-foreground">memory</span>
              <h3 className="text-xs font-semibold">Resource Tuning</h3>
            </div>
            <div className="p-4 space-y-5">
              {isLoading ? (
                <PageSpinner />
              ) : (
                <div className="space-y-5">
                  <FeatureNote title="What this does:">
                    Tune PostgreSQL memory allocation settings (shared_buffers, work_mem) to optimize how the engine caches data and processes query operations.
                  </FeatureNote>
                  <p className="text-xs text-muted-foreground">
                    Note: <code>Shared buffers</code> modification requires a cluster restart to take effect, while <code>Work memory</code> changes apply instantly on configuration reload.
                  </p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {memorySettings.map((s) => (
                      <SettingInput
                        key={s.key}
                        id={s.key}
                        label={s.label}
                        type={s.type}
                        value={values[s.key] ?? ""}
                        onChange={(v) => handleChange(s.key, v)}
                        hint={s.hint}
                        options={s.options}
                        disabled={update.isPending}
                      />
                    ))}
                  </div>
                  {memorySettings.map((s) =>
                    errors[s.key] ? (
                      <p key={`${s.key}-err`} className="text-xs font-semibold text-destructive">
                        {s.label}: {errors[s.key]}
                      </p>
                    ) : null,
                  )}
                  <div className="flex items-center gap-3 pt-2">
                    <Button
                      onClick={handleSave}
                      disabled={!dirty || update.isPending}
                      size="sm"
                    >
                      {update.isPending ? "Saving..." : "Apply Settings"}
                    </Button>
                    {saved && (
                      <span className="text-xs font-semibold text-emerald-600 dark:text-emerald-400">Settings queued for all nodes.</span>
                    )}
                    {update.isError && (
                      <span className="text-xs font-semibold text-destructive">
                        {update.error instanceof Error ? update.error.message : "Failed to update settings"}
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}

        {activeSubmenu === "connectivity" && (
          <div className="v-card rounded-lg overflow-hidden">
            <div className="px-4 py-3 border-b border-border flex items-center gap-2 text-foreground">
              <span className="material-symbols-outlined text-lg text-foreground">hub</span>
              <h3 className="text-xs font-semibold">Connectivity & WAL</h3>
            </div>
            <div className="p-4 space-y-5">
              {isLoading ? (
                <PageSpinner />
              ) : (
                <div className="space-y-5">
                  <FeatureNote title="What this does:">
                    Configure client connections limits and write-ahead log parameters (wal_level, max_wal_senders, max_connections).
                  </FeatureNote>
                  <p className="text-xs text-muted-foreground">
                    Note: All connections and WAL changes require a PostgreSQL service restart to take effect on cluster nodes.
                  </p>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    {connectivitySettings.map((s) => (
                      <SettingInput
                        key={s.key}
                        id={s.key}
                        label={s.label}
                        type={s.type}
                        value={values[s.key] ?? ""}
                        onChange={(v) => handleChange(s.key, v)}
                        hint={s.hint}
                        options={s.options}
                        disabled={update.isPending}
                      />
                    ))}
                  </div>
                  {connectivitySettings.map((s) =>
                    errors[s.key] ? (
                      <p key={`${s.key}-err`} className="text-xs font-semibold text-destructive">
                        {s.label}: {errors[s.key]}
                      </p>
                    ) : null,
                  )}
                  <div className="flex items-center gap-3 pt-2">
                    <Button
                      onClick={handleSave}
                      disabled={!dirty || update.isPending}
                      size="sm"
                    >
                      {update.isPending ? "Saving..." : "Apply Settings"}
                    </Button>
                    {saved && (
                      <span className="text-xs font-semibold text-emerald-600 dark:text-emerald-400">Settings queued for all nodes.</span>
                    )}
                    {update.isError && (
                      <span className="text-xs font-semibold text-destructive">
                        {update.error instanceof Error ? update.error.message : "Failed to update settings"}
                      </span>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
