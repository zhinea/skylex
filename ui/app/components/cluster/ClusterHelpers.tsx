import { useState, type ReactNode } from "react";
import { Badge } from "~/components/Badge";
import { Button } from "~/components/ui/button";

export function PgStatusBadges({
  installed,
  version,
  dataInitialized,
}: {
  installed: boolean;
  version: string;
  dataInitialized: boolean;
}) {
  if (!installed) {
    return <Badge label="not installed" />;
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <Badge label={version || "installed"} className="bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" />
      <Badge
        label={dataInitialized ? "data ready" : "not initialized"}
        className={dataInitialized ? "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" : "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800"}
      />
    </span>
  );
}

export function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <Button
      variant="outline"
      size="xs"
      onClick={handleCopy}
      title="Copy to clipboard"
      className="ml-2 h-6 px-1.5 text-[10px] uppercase font-semibold tracking-wider hover:bg-muted"
    >
      {copied ? "Copied" : "Copy"}
    </Button>
  );
}

export function ConnectionRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start gap-2 py-2 border-b border-border/40 last:border-b-0">
      <dt className="w-36 shrink-0 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground pt-1">{label}</dt>
      <dd className="flex-1 min-w-0 flex items-center gap-1">
        <code className="text-xs font-mono text-foreground break-all select-all bg-muted/30 px-1.5 py-0.5 rounded border border-border/40">{value}</code>
        <CopyButton text={value} />
      </dd>
    </div>
  );
}

export function FeatureNote({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-muted/20 px-3 py-2 text-xs text-muted-foreground flex items-start gap-1.5 leading-normal">
      <span className="font-semibold text-foreground shrink-0">{title}</span>
      <span>{children}</span>
    </div>
  );
}

export function RoleStatusBadge({ status }: { status: string }) {
  const normalized = (status || "pending").toLowerCase();
  let label = status || "pending";
  if (normalized === "ready") {
    return <Badge label="ready" className="bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50" />;
  }
  if (normalized === "failed") {
    return <Badge label="failed" className="bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50" />;
  }
  if (normalized === "deleting") {
    return <Badge label="deleting" className="bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800" />;
  }
  return <Badge label={label} className="bg-amber-50/60 text-amber-700 border-amber-200/50 dark:bg-amber-950/20 dark:text-amber-400 dark:border-amber-800/50" />;
}

export function levelColor(level: string): string {
  switch (level.toLowerCase()) {
    case "error":
      return "text-red-400 font-semibold";
    case "warn":
      return "text-amber-400 font-semibold";
    case "info":
      return "text-emerald-400 font-semibold";
    default:
      return "text-zinc-400";
  }
}

export function statusDetailColor(detail: string): string {
  switch (detail) {
    case "healthy":
    case "running":
      return "bg-emerald-50/60 text-emerald-700 border-emerald-200/50 dark:bg-emerald-950/20 dark:text-emerald-400 dark:border-emerald-800/50 border";
    case "syncing_replica":
      return "bg-blue-50/60 text-blue-700 border-blue-200/50 dark:bg-blue-900/20 dark:text-blue-400 dark:border-blue-800/50 border";
    case "initializing_data_directory":
      return "bg-amber-50/60 text-amber-700 border-amber-200/50 dark:bg-amber-950/20 dark:text-amber-400 dark:border-amber-800/50 border";
    case "installation_conflict":
    case "waiting_for_postgres":
      return "bg-rose-50/60 text-rose-700 border-rose-200/50 dark:bg-rose-950/20 dark:text-rose-400 dark:border-rose-800/50 border";
    case "stopped":
      return "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800 border";
    default:
      return "bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800 border";
  }
}

export function libpqSSLMode(mode: string) {
  if (mode === "disabled") return "disable";
  if (mode === "required") return "require";
  return mode || "disable";
}

export function connectionURI(host: string, port: number, roleName: string, sslMode: string, password?: string) {
  const user = roleName || "<user>";
  const pass = password || "<password>";
  return `postgresql://${user}:${pass}@${host}:${port}/postgres?sslmode=${libpqSSLMode(sslMode)}`;
}

export function databaseConnectionURI(host: string, port: number, databaseName: string, sslMode: string, roleName?: string, password?: string) {
  const user = roleName || "<user>";
  const pass = password || "<password>";
  return `postgresql://${user}:${pass}@${host}:${port}/${databaseName}?sslmode=${libpqSSLMode(sslMode)}`;
}

export function databasePsqlCommand(host: string, port: number, databaseName: string, sslMode: string, roleName?: string) {
  return `psql "host=${host} port=${port} dbname=${databaseName} user=${roleName || "<user>"} sslmode=${libpqSSLMode(sslMode)}"`;
}
