import type { Node } from "~/hooks/useNodes";
import type { CommandLog } from "~/hooks/useCommandLogs";
import { Card, CardHeader, CardTitle, CardContent } from "~/components/ui/card";
import { Table, TableHeader, TableRow, TableHead, TableBody, TableCell } from "~/components/ui/table";
import { RefreshCw } from "lucide-react";
import { FeatureNote, PgStatusBadges, levelColor } from "./ClusterHelpers";

interface InstallationProgressCardProps {
  nodes: Node[];
  logs: CommandLog[];
}

export function InstallationProgressCard({ nodes, logs }: InstallationProgressCardProps) {
  const nodeList = nodes;
  const totalNodes = nodeList.length;
  const readyNodes = nodeList.filter((n) => n.postgresInstalled && n.postgresDataInitialized);
  const progressPct = totalNodes > 0 ? Math.round((readyNodes.length / totalNodes) * 100) : 0;
  const tailLogs = logs.slice(-12);

  return (
    <Card className="shadow-xs">
      <CardHeader className="border-b border-border/60 pb-4">
        <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
          <RefreshCw className="size-4 text-muted-foreground animate-spin-slow" />
          Installation Progress
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-5 pt-6">
        <FeatureNote title="What this does:">
          Track the setup steps Skylex runs on each node, such as installing PostgreSQL, initializing data, starting the service, and preparing replication.
        </FeatureNote>
        <div>
          <div className="flex justify-between text-xs mb-2">
            <span className="font-semibold text-muted-foreground uppercase tracking-wider">Provisioning</span>
            <span className="text-foreground font-semibold">
              {readyNodes.length}/{totalNodes} nodes ready
            </span>
          </div>
          <div className="w-full bg-muted rounded-full h-2 overflow-hidden border border-border/50">
            <div
              className={`h-2 rounded-full transition-all duration-500 ${
                progressPct === 100 ? "bg-emerald-500" : progressPct > 0 ? "bg-primary" : "bg-amber-500"
              }`}
              style={{ width: `${progressPct}%` }}
            />
          </div>
        </div>

        {nodeList.length > 0 && (
          <div className="overflow-x-auto rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/30">
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Node</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Location</TableHead>
                  <TableHead className="h-9 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4">Install State</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodeList.map((n) => (
                  <TableRow key={n.id} className="hover:bg-muted/10 border-b border-border/40 last:border-none">
                    <TableCell className="px-4 py-2.5">
                      <div className="text-foreground font-semibold">{n.hostname}</div>
                      <div className="text-[10px] text-muted-foreground uppercase tracking-wider font-mono">{n.role}</div>
                    </TableCell>
                    <TableCell className="px-4 py-2.5 text-foreground/80 text-xs">
                      {n.serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Dockerized" : "Native"}
                    </TableCell>
                    <TableCell className="px-4 py-2.5">
                      <PgStatusBadges
                        installed={n.postgresInstalled}
                        version={n.postgresVersion}
                        dataInitialized={n.postgresDataInitialized}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}

        <div className="space-y-2">
          <div className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">
            Recent Installation Logs
          </div>
          {tailLogs.length === 0 ? (
            <p className="text-xs text-muted-foreground">Logs appear once agents start executing installation commands.</p>
          ) : (
            <div className="max-h-48 overflow-y-auto font-mono text-[11px] rounded-lg bg-zinc-950 text-zinc-200 border border-zinc-800 p-3 space-y-1">
              {tailLogs.map((log) => (
                <div key={log.id} className="grid grid-cols-[5rem_7rem_1fr] gap-2 py-0.5 border-b border-zinc-900/50 last:border-b-0 leading-relaxed">
                  <span className="text-zinc-500">{new Date(Number(log.timestampMs)).toLocaleTimeString()}</span>
                  <span className="text-zinc-400 truncate">{log.hostname || log.nodeId?.slice(0, 8) || "-"}</span>
                  <span className={`${levelColor(log.level)} break-all`}>{log.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
