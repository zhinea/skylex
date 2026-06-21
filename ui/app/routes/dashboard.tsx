import { useClusters } from "~/hooks/useClusters";
import { Badge } from "~/components/Badge";
import { PageSpinner } from "~/components/Spinner";
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
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { PlusIcon, Layers } from "lucide-react";

export default function DashboardPage() {
  const { data, isLoading } = useClusters(1, 100);

  if (isLoading) return <PageSpinner />;

  const clusters = data?.clusters || [];
  const totalClusters = data?.pagination?.total || clusters.length;
  const healthyClusters = clusters.filter((c) => c.status === "HEALTHY").length;
  const degradedClusters = clusters.filter((c) => c.status === "DEGRADED").length;
  const failedClusters = clusters.filter((c) => c.status === "FAILED").length;

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Dashboard</h2>
          <p className="text-xs text-muted-foreground mt-1">Overview of your PostgreSQL clusters and active nodes.</p>
        </div>
        <Button asChild variant="default" size="sm">
          <Link to="/clusters/create" className="flex items-center gap-1.5">
            <PlusIcon className="size-3.5" />
            Create Cluster
          </Link>
        </Button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card className="shadow-xs hover:border-foreground/20 transition-all duration-200">
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Total Clusters</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold tracking-tight text-foreground">{totalClusters}</div>
          </CardContent>
        </Card>

        <Card className="shadow-xs hover:border-emerald-500/20 transition-all duration-200">
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Healthy</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold tracking-tight text-emerald-600 dark:text-emerald-400">{healthyClusters}</div>
          </CardContent>
        </Card>

        <Card className="shadow-xs hover:border-amber-500/20 transition-all duration-200">
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Degraded</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold tracking-tight text-amber-600 dark:text-amber-400">{degradedClusters}</div>
          </CardContent>
        </Card>

        <Card className="shadow-xs hover:border-destructive/20 transition-all duration-200">
          <CardHeader className="pb-2">
            <CardTitle className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Failed</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold tracking-tight text-destructive">{failedClusters}</div>
          </CardContent>
        </Card>
      </div>

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <Layers className="size-4 text-muted-foreground" />
            Recent Clusters
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
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Status</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Replicas</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {clusters.slice(0, 10).map((c) => (
                    <TableRow key={c.id} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5">
                        <Link
                          to={`/clusters/${c.id}`}
                          className="text-foreground hover:underline font-semibold"
                        >
                          {c.name}
                        </Link>
                      </TableCell>
                      <TableCell className="text-foreground/95 px-6 py-3.5 text-xs font-mono">
                        {c.config?.engine || "POSTGRESQL"} {c.config?.version || "16"}
                      </TableCell>
                      <TableCell className="px-6 py-3.5">
                        <Badge label={c.status} />
                      </TableCell>
                      <TableCell className="text-foreground/80 px-6 py-3.5 text-xs font-medium">
                        {c.config?.replicaCount || 0}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}