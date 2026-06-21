import { useState, useMemo, useEffect } from "react";
import { useClusterSettings, useUpdateClusterSettings } from "~/hooks/useClusterSettings";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import { SettingInput, curatedSettings, validateSettingValue } from "~/components/SettingInput";
import { FeatureNote } from "./ClusterHelpers";
import { PageSpinner } from "~/components/Spinner";
import { Settings as SettingsIcon } from "lucide-react";

interface SettingsCardProps {
  clusterId: string;
}

export function SettingsCard({ clusterId }: SettingsCardProps) {
  const { data, isLoading } = useClusterSettings(clusterId);
  const update = useUpdateClusterSettings();
  const [values, setValues] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string | null>>({});
  const [saved, setSaved] = useState(false);

  const parameters = useMemo(() => data?.settings?.parameters ?? {}, [data?.settings?.parameters]);

  useEffect(() => {
    const next: Record<string, string> = {};
    for (const s of curatedSettings) {
      next[s.key] = parameters[s.key] ?? "";
    }
    setValues(next);
  }, [parameters]);

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
          setTimeout(() => setSaved(false), 3000);
        },
      },
    );
  }

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <SettingsIcon className="size-4 text-muted-foreground" />
          PostgreSQL Settings
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        {isLoading ? (
          <PageSpinner />
        ) : (
          <div className="space-y-5">
            <FeatureNote title="What this does:">
              Tune common PostgreSQL behavior without editing configuration files by hand. Skylex validates values, saves them, and queues the needed reload or restart on cluster nodes.
            </FeatureNote>
            <p className="text-xs text-muted-foreground">
              Start with defaults unless you know a setting solves a specific workload problem. Some settings apply with a quick reload; others need PostgreSQL to restart.
            </p>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {curatedSettings.map((s) => (
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
            {curatedSettings.map((s) =>
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
      </CardContent>
    </Card>
  );
}
