import { useState } from "react";
import { useNavigate } from "react-router";
import { useClusters } from "~/hooks/useClusters";
import { useBackups, useCreateRestoreJob } from "~/hooks/useBackups";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { PageSpinner } from "~/components/Spinner";
import { Button } from "~/components/ui/button";
import { RefreshCw, ArrowLeft } from "lucide-react";

export default function RestorePage() {
  const navigate = useNavigate();
  const { data: clustersData, isLoading: clustersLoading } = useClusters(1, 100);
  const [selectedCluster, setSelectedCluster] = useState("");
  const { data: backupsData, isLoading: backupsLoading } = useBackups(selectedCluster || undefined);
  const restoreJob = useCreateRestoreJob();
  const [selectedBackup, setSelectedBackup] = useState("");
  const [targetTime, setTargetTime] = useState("");
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  if (clustersLoading) return <PageSpinner />;

  const clusters = clustersData?.clusters || [];
  const backups = backupsData?.backups || [];

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSuccess("");
    try {
      await restoreJob.mutateAsync({
        clusterId: selectedCluster,
        backupId: selectedBackup,
        targetTime: targetTime || undefined,
      });
      setSuccess("Restore job created successfully.");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create restore job");
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Restore Database</h2>
          <p className="text-xs text-muted-foreground mt-1">Perform Point-in-Time Recovery (PITR) or full backup restores.</p>
        </div>
        <Button asChild variant="outline" size="sm">
          <button onClick={() => navigate("/backups")} className="flex items-center gap-1.5">
            <ArrowLeft className="size-3.5" />
            Back to Backups
          </button>
        </Button>
      </div>

      <div className="max-w-lg">
        <Card className="shadow-xs">
          <CardHeader className="border-b border-border/60 pb-4">
            <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
              <RefreshCw className="size-4 text-muted-foreground" />
              Point-in-Time Recovery Details
            </CardTitle>
          </CardHeader>
          <CardContent className="pt-6">
            {backups.length === 0 && selectedCluster && !backupsLoading ? (
              <div className="space-y-4">
                <p className="text-sm text-muted-foreground">
                  No backups available for this cluster. Create a backup first.
                </p>
                <Button variant="outline" size="sm" onClick={() => setSelectedCluster("")}>
                  Choose Different Cluster
                </Button>
              </div>
            ) : (
              <form onSubmit={handleSubmit} className="space-y-5">
                {error && (
                  <div className="bg-destructive/10 border border-destructive/20 text-destructive px-3 py-2.5 rounded-lg text-xs font-medium">
                    {error}
                  </div>
                )}
                {success && (
                  <div className="bg-emerald-50/60 border border-emerald-200/50 text-emerald-700 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50 px-3 py-2.5 rounded-lg text-xs font-medium">
                    {success}
                  </div>
                )}
                
                <div className="space-y-1.5">
                  <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Source Cluster</label>
                  <select
                    value={selectedCluster}
                    onChange={(e) => { setSelectedCluster(e.target.value); setSelectedBackup(""); }}
                    required
                    className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  >
                    <option value="" className="bg-popover text-popover-foreground">Select cluster...</option>
                    {clusters.map((c) => (
                      <option key={c.id} value={c.id} className="bg-popover text-popover-foreground">{c.name}</option>
                    ))}
                  </select>
                </div>

                {backupsLoading ? (
                  <div className="py-4"><PageSpinner /></div>
                ) : backups.length > 0 && (
                  <div className="space-y-1.5 animate-in slide-in-from-top-1 duration-200">
                    <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Base Backup Snapshot</label>
                    <select
                      value={selectedBackup}
                      onChange={(e) => setSelectedBackup(e.target.value)}
                      required
                      className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                    >
                      <option value="" className="bg-popover text-popover-foreground">Select backup...</option>
                      {backups.filter((b) => b.status === "COMPLETED").map((b) => (
                        <option key={b.id} value={b.id} className="bg-popover text-popover-foreground">
                          {new Date(b.createdAt).toLocaleString()} - {b.type || "FULL"} ({b.lsn || "N/A"})
                        </option>
                      ))}
                    </select>
                  </div>
                )}

                <div className="space-y-1.5">
                  <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    Target Time (optional, for PITR)
                  </label>
                  <input
                    type="datetime-local"
                    value={targetTime}
                    onChange={(e) => setTargetTime(e.target.value)}
                    className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                  />
                  <p className="text-[10px] text-muted-foreground">
                    Specify the date and time to roll forward database logs to. Defaults to snapshot time if omitted.
                  </p>
                </div>

                <div className="flex gap-3 pt-4 border-t border-border mt-6">
                  <Button
                    type="submit"
                    disabled={restoreJob.isPending || !selectedCluster || !selectedBackup}
                    size="sm"
                  >
                    {restoreJob.isPending ? "Creating Restore Job..." : "Start Restore"}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => navigate("/backups")}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}