import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
import { useCreateCluster } from "~/hooks/useClusters";
import { useNodes } from "~/hooks/useNodes";
import { Card, CardContent, CardHeader, CardTitle } from "~/components/ui/card";
import { Badge } from "~/components/Badge";
import { PageSpinner } from "~/components/Spinner";
import { Button } from "~/components/ui/button";
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "~/components/ui/table";
import { Check, ArrowRight, AlertTriangle, Info } from "lucide-react";

const STEPS = ["Select Nodes", "Configure", "Review"] as const;

export default function CreateClusterPage() {
  const navigate = useNavigate();
  const createCluster = useCreateCluster();
  const { data: nodesData } = useNodes();

  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [engine, setEngine] = useState("POSTGRESQL");
  const [version, setVersion] = useState("16");
  const [replicaCount, setReplicaCount] = useState(0);
  const [replicationMode, setReplicationMode] = useState("ASYNC");
  const [pitrEnabled, setPitrEnabled] = useState(false);
  const [serviceLocation, setServiceLocation] = useState("SERVICE_LOCATION_NATIVE");
  // Ordered list of selected node IDs; first = primary, rest = replicas.
  const [selectedNodeIds, setSelectedNodeIds] = useState<string[]>([]);
  const [error, setError] = useState("");

  const allNodes = nodesData?.nodes ?? [];
  const idleNodes = useMemo(() => allNodes.filter((n) => !n.clusterId), [allNodes]);

  const neededNodes = replicaCount + 1;

  const toggleNode = (nodeId: string) => {
    setSelectedNodeIds((prev) => {
      if (prev.includes(nodeId)) {
        return prev.filter((id) => id !== nodeId);
      }
      return [...prev, nodeId];
    });
  };

  const canProceedFromStep0 =
    selectedNodeIds.length === neededNodes &&
    selectedNodeIds.every((id) => {
      const n = allNodes.find((n) => n.id === id);
      return n && !n.clusterId;
    });

  const canProceedFromStep1 = name.trim().length > 0;

  const handleSubmit = async () => {
    setError("");
    try {
      const result = await createCluster.mutateAsync({
        name,
        config: {
          engine,
          version,
          replicaCount,
          replicationMode,
          pitrEnabled,
          serviceLocation,
        },
        nodeIds: selectedNodeIds,
      });
      navigate(`/clusters/${result.cluster.id}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to create cluster");
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-300">
      <div>
        <h2 className="text-2xl font-bold tracking-tight text-foreground">Create Cluster</h2>
        <p className="text-xs text-muted-foreground mt-1">Set up a high-availability database cluster and provision primary and replicas.</p>
      </div>

      {/* Step indicator */}
      <div className="flex items-center gap-4 border-b border-border pb-6">
        {STEPS.map((label, i) => (
          <div key={label} className="flex items-center gap-2">
            <button
              onClick={() => (i < step ? setStep(i) : undefined)}
              disabled={i > step}
              className={`flex items-center justify-center size-7 rounded-full text-xs font-semibold transition-all duration-200
                ${i < step ? "bg-foreground text-background cursor-pointer" : ""}
                ${i === step ? "bg-primary text-primary-foreground shadow-xs ring-2 ring-primary/20" : ""}
                ${i > step ? "bg-muted text-muted-foreground border border-border" : ""}
              `}
            >
              {i < step ? <Check className="size-3.5" /> : i + 1}
            </button>
            <span
              className={`text-xs font-semibold uppercase tracking-wider ${
                i <= step
                  ? "text-foreground"
                  : "text-muted-foreground"
              }`}
            >
              {label}
            </span>
            {i < STEPS.length - 1 && (
              <ArrowRight className="size-3 text-muted-foreground/60 mx-1" />
            )}
          </div>
        ))}
      </div>

      {error && (
        <div className="bg-destructive/10 border border-destructive/20 text-destructive px-3 py-2.5 rounded-lg text-xs font-medium">
          {error}
        </div>
      )}

      {/* Step 0: Select Nodes */}
      {step === 0 && (
        <Card className="shadow-xs">
          <CardHeader className="border-b border-border/60 pb-4">
            <CardTitle className="text-sm font-semibold tracking-tight text-foreground">Step 1: Select Nodes</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6 pt-6">
            <p className="text-xs text-muted-foreground leading-relaxed max-w-2xl">
              Select <strong className="text-foreground">{neededNodes} node{neededNodes !== 1 ? "s" : ""}</strong> for
              this cluster — 1 primary + {replicaCount} replica{replicaCount !== 1 ? "s" : ""}.
              The first selected node becomes the primary. Adjust replica count in the Configure step.
            </p>

            <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              <span>Selected Nodes:</span>
              <span className={selectedNodeIds.length === neededNodes ? "text-emerald-600 dark:text-emerald-400" : "text-foreground"}>
                {selectedNodeIds.length} of {neededNodes}
              </span>
              {selectedNodeIds.length === neededNodes && (
                <span className="ml-1 text-[10px] bg-emerald-50 dark:bg-emerald-950/30 border border-emerald-200/30 px-1.5 py-0.5 rounded text-emerald-600 dark:text-emerald-400">&bull; Ready</span>
              )}
            </div>

            {allNodes.length === 0 ? (
              <div className="flex items-start gap-2.5 p-4 rounded-lg bg-amber-50/60 border border-amber-200/50 dark:bg-amber-950/20 dark:border-amber-800/50 text-amber-800 dark:text-amber-300">
                <AlertTriangle className="size-4 shrink-0 mt-0.5" />
                <p className="text-xs leading-relaxed font-medium">
                  No agents registered. Register agents in the Nodes page before creating a cluster.
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto border border-border rounded-lg">
                <Table>
                  <TableHeader>
                    <TableRow className="bg-muted/30">
                      <TableHead className="w-10 px-4 py-2"></TableHead>
                      <TableHead className="text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 py-2">Role</TableHead>
                      <TableHead className="text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 py-2">Hostname</TableHead>
                      <TableHead className="text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 py-2">Address</TableHead>
                      <TableHead className="text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 py-2">PostgreSQL</TableHead>
                      <TableHead className="text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 py-2">Docker</TableHead>
                      <TableHead className="text-xs font-semibold uppercase tracking-wider text-muted-foreground px-4 py-2">Status</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {allNodes.map((n) => {
                      const isAssigned = !!n.clusterId;
                      const isSelected = selectedNodeIds.includes(n.id);
                      const selectionIndex = selectedNodeIds.indexOf(n.id);
                      const wouldExceedLimit =
                        !isSelected && selectedNodeIds.length >= neededNodes;
                      const isDisabled = isAssigned || wouldExceedLimit;

                      let roleLabel: React.ReactNode = null;
                      if (isSelected) {
                        roleLabel =
                          selectionIndex === 0 ? (
                            <Badge label="PRIMARY" className="bg-violet-50/80 text-violet-700 dark:bg-violet-950/20 dark:text-violet-400 border-violet-200/50 dark:border-violet-800/50" />
                          ) : (
                            <Badge label="REPLICA" className="bg-neutral-50 text-neutral-600 border-neutral-200 dark:bg-neutral-900/20 dark:text-neutral-400 dark:border-neutral-800" />
                          );
                      }

                      return (
                        <TableRow
                          key={n.id}
                          className={`transition-colors border-b border-border/50
                            ${isDisabled ? "opacity-40" : "cursor-pointer hover:bg-muted/20"}
                            ${isSelected ? "bg-muted/40" : ""}
                          `}
                          onClick={() => !isDisabled && toggleNode(n.id)}
                        >
                          <TableCell className="px-4 py-2.5" onClick={(e) => e.stopPropagation()}>
                            <input
                              type="checkbox"
                              checked={isSelected}
                              disabled={isDisabled}
                              onChange={() => !isDisabled && toggleNode(n.id)}
                              className="rounded border-input text-primary focus:ring-ring shrink-0 size-3.5"
                            />
                          </TableCell>
                          <TableCell className="px-4 py-2.5">
                            {roleLabel ?? (isAssigned ? <Badge label="assigned" /> : null)}
                          </TableCell>
                          <TableCell className="px-4 py-2.5 text-foreground font-semibold">
                            {n.hostname}
                          </TableCell>
                          <TableCell className="px-4 py-2.5 text-muted-foreground text-xs font-mono">
                            {n.address}:{n.port}
                          </TableCell>
                          <TableCell className="px-4 py-2.5">
                            {n.postgresInstalled ? (
                              <span className="text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                                {n.postgresVersion || "installed"}
                              </span>
                            ) : (
                              <Badge label="not installed" />
                            )}
                          </TableCell>
                          <TableCell className="px-4 py-2.5">
                            {n.dockerAvailable ? (
                              <span className="text-xs text-emerald-600 dark:text-emerald-400 font-medium">available</span>
                            ) : (
                              <span className="text-xs text-muted-foreground font-medium">unavailable</span>
                            )}
                          </TableCell>
                          <TableCell className="px-4 py-2.5">
                            {n.statusDetail ? (
                              <span className="text-xs text-muted-foreground font-medium">
                                {n.statusDetail}
                              </span>
                            ) : (
                              <Badge label={n.clusterId ? "assigned" : "idle"} />
                            )}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
            )}

            {idleNodes.length < neededNodes && (
              <div className="flex items-start gap-2.5 p-4 rounded-lg bg-destructive/10 border border-destructive/20 text-destructive">
                <AlertTriangle className="size-4 shrink-0 mt-0.5" />
                <p className="text-xs leading-relaxed font-medium">
                  Only {idleNodes.length} idle node{idleNodes.length !== 1 ? "s" : ""} available. Need {neededNodes}.
                  Decrease replica count in step 2 or register more agents.
                </p>
              </div>
            )}

            {serviceLocation === "SERVICE_LOCATION_DOCKER" &&
              selectedNodeIds.some((id) => {
                const n = allNodes.find((n) => n.id === id);
                return n && !n.dockerAvailable;
              }) && (
                <div className="flex items-start gap-2.5 p-4 rounded-lg bg-amber-50/60 border border-amber-200/50 dark:bg-amber-950/20 dark:border-amber-800/50 text-amber-800 dark:text-amber-300">
                  <AlertTriangle className="size-4 shrink-0 mt-0.5" />
                  <p className="text-xs leading-relaxed font-medium">
                    One or more selected nodes do not have Docker available. Install Docker on those hosts
                    or switch to Native service location.
                  </p>
                </div>
              )}

            <div className="flex gap-3 pt-4 border-t border-border">
              <Button
                onClick={() => setStep(1)}
                disabled={!canProceedFromStep0}
                size="sm"
              >
                Next: Configure
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => navigate("/clusters")}
              >
                Cancel
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Step 1: Configure */}
      {step === 1 && (
        <Card className="shadow-xs">
          <CardHeader className="border-b border-border/60 pb-4">
            <CardTitle className="text-sm font-semibold tracking-tight text-foreground">Step 2: Cluster Configuration</CardTitle>
          </CardHeader>
          <CardContent className="space-y-5 pt-6 max-w-lg">
            <div className="space-y-1.5">
              <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Cluster Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                placeholder="my-cluster"
              />
            </div>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Engine</label>
                <select
                  value={engine}
                  onChange={(e) => setEngine(e.target.value)}
                  className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="POSTGRESQL" className="bg-popover text-popover-foreground">PostgreSQL</option>
                </select>
              </div>
              <div className="space-y-1.5">
                <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Version</label>
                <select
                  value={version}
                  onChange={(e) => setVersion(e.target.value)}
                  className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="16" className="bg-popover text-popover-foreground">16</option>
                  <option value="15" className="bg-popover text-popover-foreground">15</option>
                  <option value="14" className="bg-popover text-popover-foreground">14</option>
                </select>
              </div>
            </div>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Replicas</label>
                <input
                  type="number"
                  value={replicaCount}
                  onChange={(e) => {
                    const maxReplicas = Math.max(0, idleNodes.length - 1);
                    const val = Math.max(0, Math.min(maxReplicas, Number(e.target.value) || 0));
                    const newNeeded = val + 1;
                    setReplicaCount(val);
                    setSelectedNodeIds((prev) => prev.slice(0, newNeeded));
                  }}
                  min={0}
                  max={Math.max(0, idleNodes.length - 1)}
                  className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                />
                {replicaCount > idleNodes.length - 1 && (
                  <p className="text-[10px] text-destructive font-semibold">
                    Max replicas available: {Math.max(0, idleNodes.length - 1)}.
                  </p>
                )}
              </div>
              <div className="space-y-1.5">
                <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Replication</label>
                <select
                  value={replicationMode}
                  onChange={(e) => setReplicationMode(e.target.value)}
                  className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
                >
                  <option value="ASYNC" className="bg-popover text-popover-foreground">Asynchronous</option>
                  <option value="SYNC" className="bg-popover text-popover-foreground">Synchronous</option>
                </select>
              </div>
            </div>

            <div className="flex items-center gap-2 pt-1.5">
              <input
                type="checkbox"
                id="pitr"
                checked={pitrEnabled}
                onChange={(e) => setPitrEnabled(e.target.checked)}
                className="rounded border-input text-primary focus:ring-ring"
              />
              <label htmlFor="pitr" className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Enable PITR (Point-in-Time Recovery)</label>
            </div>

            <div className="space-y-1.5 pt-1.5">
              <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground">Service Location</label>
              <select
                value={serviceLocation}
                onChange={(e) => setServiceLocation(e.target.value)}
                className="w-full px-3 py-2 text-sm border rounded-md bg-transparent border-input text-foreground transition-all focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
              >
                <option value="SERVICE_LOCATION_NATIVE" className="bg-popover text-popover-foreground">Native (run PostgreSQL directly on host)</option>
                <option value="SERVICE_LOCATION_DOCKER" className="bg-popover text-popover-foreground">Dockerized (run PostgreSQL inside Docker)</option>
              </select>
              <p className="text-[10px] text-muted-foreground leading-normal">
                Native runs the PostgreSQL process directly on the host. Dockerized wraps it in a container.
              </p>
            </div>

            {selectedNodeIds.length !== neededNodes && (
              <div className="flex items-start gap-2.5 p-4 rounded-lg bg-amber-50/60 border border-amber-200/50 dark:bg-amber-950/20 dark:border-amber-800/50 text-amber-800 dark:text-amber-300">
                <AlertTriangle className="size-4 shrink-0 mt-0.5" />
                <p className="text-xs leading-relaxed font-medium">
                  Replica count changed — you need to re-select nodes (need {neededNodes}, currently have {selectedNodeIds.length} selected).
                  Go back to update your selection.
                </p>
              </div>
            )}

            <div className="flex gap-3 pt-4 border-t border-border mt-6">
              <Button variant="outline" size="sm" onClick={() => setStep(0)}>
                Back
              </Button>
              <Button
                onClick={() => setStep(2)}
                disabled={!canProceedFromStep1 || selectedNodeIds.length !== neededNodes}
                size="sm"
              >
                Next: Review
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Step 2: Review */}
      {step === 2 && (
        <Card className="shadow-xs">
          <CardHeader className="border-b border-border/60 pb-4">
            <CardTitle className="text-sm font-semibold tracking-tight text-foreground">Step 3: Review & Create</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6 pt-6 max-w-lg">
            <dl className="space-y-3 text-sm divide-y divide-border">
              <div className="flex justify-between pb-1">
                <dt className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Cluster Name</dt>
                <dd className="text-foreground font-semibold">{name}</dd>
              </div>
              <div className="flex justify-between pt-3 pb-1">
                <dt className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Engine</dt>
                <dd className="text-foreground font-medium text-xs font-mono">{engine} {version}</dd>
              </div>
              <div className="flex justify-between pt-3 pb-1">
                <dt className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Nodes Configuration</dt>
                <dd className="text-foreground font-medium text-xs">
                  1 primary + {replicaCount} replica{replicaCount !== 1 ? "s" : ""} ({neededNodes} total)
                </dd>
              </div>
              <div className="flex justify-between pt-3 pb-1">
                <dt className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Replication</dt>
                <dd className="text-foreground font-medium text-xs">
                  {replicationMode === "SYNC" ? "Synchronous" : "Asynchronous"}
                </dd>
              </div>
              <div className="flex justify-between pt-3 pb-1">
                <dt className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">PITR</dt>
                <dd className="text-foreground font-medium text-xs">{pitrEnabled ? "Enabled" : "Disabled"}</dd>
              </div>
              <div className="flex justify-between pt-3 pb-1">
                <dt className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">Service Location</dt>
                <dd className="text-foreground font-medium text-xs">
                  {serviceLocation === "SERVICE_LOCATION_DOCKER" ? "Dockerized" : "Native"}
                </dd>
              </div>
            </dl>

            {/* Node assignment summary */}
            <div className="border border-border rounded-lg overflow-hidden">
              <div className="px-4 py-2 bg-muted/40 text-[10px] font-semibold text-muted-foreground uppercase tracking-wider border-b border-border">
                Node Assignment Summary
              </div>
              <Table>
                <TableBody>
                  {selectedNodeIds.map((nodeId, i) => {
                    const n = allNodes.find((n) => n.id === nodeId);
                    return (
                      <TableRow key={nodeId} className="border-b border-border/50 last:border-none">
                        <TableCell className="px-4 py-2.5 text-foreground font-semibold">
                          {n?.hostname ?? nodeId}
                        </TableCell>
                        <TableCell className="px-4 py-2.5 text-right">
                          <Badge label={i === 0 ? "PRIMARY" : "REPLICA"} className={i === 0 ? "bg-violet-50/80 text-violet-700 dark:bg-violet-950/20 dark:text-violet-400 border-violet-200/50 dark:border-violet-800/50" : ""} />
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>

            <div className="flex items-start gap-2.5 p-4 rounded-lg bg-muted/20 border border-border">
              <Info className="size-4 text-muted-foreground shrink-0 mt-0.5" />
              <p className="text-xs leading-relaxed text-muted-foreground">
                After creation, Skylex will queue preflight, installation/provisioning, and initialization
                commands for every selected node. Native nodes install PostgreSQL when missing; Dockerized
                clusters pull and run the official Postgres container.
              </p>
            </div>

            <div className="flex gap-3 pt-4 border-t border-border mt-6">
              <Button variant="outline" size="sm" onClick={() => setStep(1)}>
                Back
              </Button>
              <Button
                onClick={handleSubmit}
                disabled={createCluster.isPending}
                size="sm"
                className="bg-emerald-600 hover:bg-emerald-700 dark:bg-emerald-500 dark:hover:bg-emerald-600 text-white"
              >
                {createCluster.isPending ? "Creating..." : "Create Cluster"}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
