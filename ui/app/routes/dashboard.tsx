import { useClusters } from "~/hooks/useClusters";
import { Badge } from "~/components/Badge";
import { Card } from "~/components/Card";
import { PageSpinner } from "~/components/Spinner";
import { Link } from "react-router";
import { buttonVariants } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { Card as ShadcnCard, CardContent, CardHeader, CardTitle } from "~/components/ui/card";

export default function DashboardPage() {
  const { data, isLoading } = useClusters(1, 100);

  if (isLoading) return <PageSpinner />;

  const clusters = data?.clusters || [];
  const totalClusters = data?.pagination?.total || clusters.length;
  const healthyClusters = clusters.filter((c) => c.status === "HEALTHY").length;
  const degradedClusters = clusters.filter((c) => c.status === "DEGRADED").length;
  const failedClusters = clusters.filter((c) => c.status === "FAILED").length;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold text-foreground">Dashboard</h2>
        <Link
          to="/clusters/create"
          className={buttonVariants({ variant: "default" })}
        >
          Create Cluster
        </Link>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <ShadcnCard>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Total Clusters</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-foreground">{totalClusters}</div>
          </CardContent>
        </ShadcnCard>

        <ShadcnCard>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Healthy</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-emerald-600 dark:text-emerald-400">{healthyClusters}</div>
          </CardContent>
        </ShadcnCard>

        <ShadcnCard>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Degraded</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-amber-600 dark:text-amber-400">{degradedClusters}</div>
          </CardContent>
        </ShadcnCard>

        <ShadcnCard>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Failed</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold text-destructive">{failedClusters}</div>
          </CardContent>
        </ShadcnCard>
      </div>

      <Card title="Recent Clusters">
        {clusters.length === 0 ? (
          <p className="text-sm text-muted-foreground py-8 text-center">
            No clusters yet. Create your first cluster to get started.
          </p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Engine</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Replicas</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {clusters.slice(0, 10).map((c) => (
                <TableRow key={c.id}>
                  <TableCell>
                    <Link
                      to={`/clusters/${c.id}`}
                      className="text-primary hover:underline font-medium"
                    >
                      {c.name}
                    </Link>
                  </TableCell>
                  <TableCell className="text-foreground">
                    {c.config?.engine || "POSTGRESQL"}
                  </TableCell>
                  <TableCell>
                    <Badge label={c.status} />
                  </TableCell>
                  <TableCell className="text-foreground">
                    {c.config?.replicaCount || 0}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Card>
    </div>
  );
}