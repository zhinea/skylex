import { useState } from "react";
import { useAuditLogs } from "~/hooks/useAuditLogs";
import { PageSpinner } from "~/components/Spinner";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { FileText } from "lucide-react";

export default function AuditPage() {
  const [page, setPage] = useState(1);
  const { data, isLoading } = useAuditLogs(page);

  if (isLoading) return <PageSpinner />;

  const entries = data?.entries || [];
  const total = data?.pagination?.total || 0;
  const pageSize = data?.pagination?.pageSize || 50;

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-foreground">Audit Logs</h2>
          <p className="text-xs text-muted-foreground mt-1">Review system event histories, administrative changes, and agent commands.</p>
        </div>
      </div>

      <Card className="shadow-xs">
        <CardHeader className="border-b border-border/60 pb-4">
          <CardTitle className="text-sm font-semibold tracking-tight text-foreground flex items-center gap-2">
            <FileText className="size-4 text-muted-foreground" />
            Audit Records ({total})
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {entries.length === 0 ? (
            <div className="py-16 text-center">
              <p className="text-sm text-muted-foreground">
                No audit log entries yet.
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Timestamp</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">User</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Action</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Resource</TableHead>
                    <TableHead className="h-10 text-xs font-semibold uppercase tracking-wider text-muted-foreground px-6">Detail</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {entries.map((entry, i) => (
                    <TableRow key={String(entry.id) || String(i)} className="hover:bg-muted/30 transition-colors">
                      <TableCell className="px-6 py-3.5 text-muted-foreground text-xs font-mono whitespace-nowrap">
                        {entry.timestamp ? new Date(entry.timestamp).toLocaleString() : "-"}
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-foreground/90 text-xs font-mono">
                        {(entry.userId || "").substring(0, 8) || "-"}
                      </TableCell>
                      <TableCell className="px-6 py-3.5 text-foreground font-semibold">{entry.action || "-"}</TableCell>
                      <TableCell className="px-6 py-3.5 text-foreground font-medium text-xs font-mono">{entry.resource || "-"}</TableCell>
                      <TableCell className="px-6 py-3.5 text-muted-foreground text-xs max-w-md truncate" title={entry.detail}>
                        {entry.detail || "-"}
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
    </div>
  );
}