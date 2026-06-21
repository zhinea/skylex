import { useState } from "react";
import { Link } from "react-router";
import { useClusters, useDeleteCluster, useFailoverCluster } from "~/hooks/useClusters";
import { Badge } from "~/components/Badge";
import { PageSpinner } from "~/components/Spinner";
import { ConfirmDialog } from "~/components/ConfirmDialog";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { PlusIcon, Server, RefreshCw, Trash2 } from "lucide-react";

export default function ClustersPage() {
  const [page, setPage] = useState(1);
  const { data, isLoading } = useClusters(page);
  const deleteCluster = useDeleteCluster();
  const failoverCluster = useFailoverCluster();
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [failoverId, setFailoverId] = useState<string | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  if (isLoading) return <PageSpinner />;

  const clusters = data?.clusters || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 20;

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Clusters</h2>
          <p className="text-xs text-muted-foreground mt-1">Manage database high-availability clusters and failovers.</p>
        </div>
        <Button asChild variant="default" size="sm">
          <Link to="/clusters/create" className="flex items-center gap-1.5">
            <PlusIcon className="size-3.5" />
            Create Cluster
          </Link>
        </Button>
      </div>

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Server className="size-4 text-muted-foreground" />
            Active Clusters ({total})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {clusters.length === 0 ? (
            <div className="py-12 text-center">
              <p className="text-sm text-muted-foreground">
                No clusters yet. Create your first cluster to get started.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Name</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Engine</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Version</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Replicas</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Status</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6 text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clusters.map((c) => (
                    <TableRow key={c.id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5">
                        <Link to={`/clusters/${c.id}`} className="text-foreground hover:underline font-semibold">
                          {c.name}
                        </Link>
                      </TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs font-mono">{c.config?.engine || "POSTGRESQL"}</TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs font-mono">{c.config?.version || "16"}</TableCell>
                      <TableCell className="text-foreground px-6 py-3.5 text-xs font-medium">{c.config?.replicaCount || 0}</TableCell>
                      <TableCell className="px-6 py-3.5"><Badge label={c.status} /></TableCell>
                      <TableCell className="px-6 py-3.5 text-right">
                        <div className="flex items-center justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setFailoverId(c.id)}
                            className="text-amber-600 hover:text-amber-700 hover:bg-amber-50 dark:hover:bg-amber-950/20 text-xs font-medium h-7 px-2"
                            title="Failover"
                          >
                            <RefreshCw className="size-3 mr-1" />
                            Failover
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDeleteId(c.id)}
                            className="text-destructive hover:text-destructive hover:bg-destructive/10 text-xs font-medium h-7 px-2"
                          >
                            <Trash2 className="size-3 mr-1" />
                            Delete
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          {total > pageSize && (
            <div className="flex items-center justify-between px-6 py-4 border-t border-border bg-muted/20">
              <span className="text-xs text-muted-foreground">
                Page {page} of {Math.ceil(total / pageSize)}
              </span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page === 1}
                  className="h-8 text-xs"
                >
                  Prev
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPage((p) => p + 1)}
                  disabled={page >= Math.ceil(total / pageSize)}
                  className="h-8 text-xs"
                >
                  Next
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={!!deleteId}
        title="Delete Cluster"
        message={deleteError || "PostgreSQL must be stopped before deletion. Pause the cluster first if any node is still running, then delete. This action cannot be undone."}
        confirmLabel="Delete"
        onConfirm={() => {
          if (deleteId) {
            setDeleteError(null);
            deleteCluster.mutate(deleteId, {
              onSuccess: () => setDeleteId(null),
              onError: (err) => {
                const message = err instanceof Error ? err.message : "Failed to delete cluster";
                setDeleteError(message.includes("running") || message.includes("pause") || message.includes("stop")
                  ? `${message} Pause/stop the service first, wait for nodes to show stopped, then delete again.`
                  : message);
              },
            });
          }
        }}
        onCancel={() => { setDeleteId(null); setDeleteError(null); }}
      />

      <ConfirmDialog
        open={!!failoverId}
        title="Failover Cluster"
        message="This will trigger a manual failover. The current replica will be promoted to primary."
        confirmLabel="Failover"
        onConfirm={() => { if (failoverId) { failoverCluster.mutate(failoverId); setFailoverId(null); }}}
        onCancel={() => setFailoverId(null)}
      />
    </div>
  );
}

