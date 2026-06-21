import { useState } from "react";
import { useClusters } from "~/hooks/useClusters";
import { useBackups, useCreateBackup, useDeleteBackup } from "~/hooks/useBackups";
import { Badge } from "~/components/Badge";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { Link } from "react-router";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { History, RefreshCw, Trash2 } from "lucide-react";

export default function BackupsPage() {
  const [selectedCluster, setSelectedCluster] = useState("");
  const { data: clustersData } = useClusters(1, 100);
  const { data, isLoading } = useBackups(selectedCluster || undefined);
  const createBackup = useCreateBackup();
  const deleteBackup = useDeleteBackup();
  const [deleteId, setDeleteId] = useState<string | null>(null);

  const clusters = clustersData?.clusters || [];
  const backups = data?.backups || [];

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Backups</h2>
          <p className="text-xs text-muted-foreground mt-1">Manage database backups, snapshots, and point-in-time recovery archives.</p>
        </div>
        <div className="flex items-center gap-3">
          <select
            value={selectedCluster}
            onChange={(e) => setSelectedCluster(e.target.value)}
            className="px-3 py-1.5 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="" className="bg-popover text-popover-foreground">All Clusters</option>
            {clusters.map((c) => (
              <option key={c.id} value={c.id} className="bg-popover text-popover-foreground">{c.name}</option>
            ))}
          </select>
          <Button asChild variant="default" size="sm">
            <Link to="/restore" className="flex items-center gap-1">
              <RefreshCw className="size-3.5" />
              Restore
            </Link>
          </Button>
        </div>
      </div>

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <History className="size-4 text-muted-foreground" />
            Backup Snapshots ({backups.length})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="py-12"><PageSpinner /></div>
          ) : backups.length === 0 ? (
            <div className="py-16 text-center">
              <p className="text-sm text-muted-foreground">
                No backups available. Create a cluster and enable PITR to see backups here.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Cluster</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Type</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Size</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">LSN</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Status</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Created</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {backups.map((b) => (
                    <TableRow key={b.id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5 text-foreground font-semibold">
                        {clusters.find((c) => c.id === b.clusterId)?.name || b.clusterId.substring(0, 8)}
                      </TableCell>
                      <TableCell className="px-6 py-3.5"><Badge label={b.type || "FULL"} /></TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs font-medium">
                        {b.sizeBytes ? `${(Number(b.sizeBytes) / 1024 / 1024).toFixed(1)} MB` : "-"}
                      </TableCell>
                      <TableCell className="text-muted-foreground px-6 py-3.5 text-xs font-mono">{b.lsn || "-"}</TableCell>
                      <TableCell className="px-6 py-3.5"><Badge label={b.status} /></TableCell>
                      <TableCell className="text-muted-foreground px-6 py-3.5 text-xs">
                        {new Date(b.createdAt).toLocaleString()}
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-right">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setDeleteId(b.id)}
                          className="text-destructive hover:text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                        >
                          <Trash2 className="size-3 mr-1" />
                          Delete
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={!!deleteId}
        title="Delete Backup"
        message="Are you sure you want to delete this backup?"
        confirmLabel="Delete"
        onConfirm={() => { if (deleteId) { deleteBackup.mutate(deleteId); setDeleteId(null); }}}
        onCancel={() => setDeleteId(null)}
      />
    </div>
  );
}