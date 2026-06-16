import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "~/lib/api";

interface Backup {
  id: string;
  clusterId: string;
  nodeId: string;
  type: string;
  storagePath: string;
  walStart: string;
  walStop: string;
  lsn: string;
  sizeBytes: string;
  status: string;
  createdAt: string;
  completedAt: string;
}

interface RestoreJob {
  id: string;
  clusterId: string;
  backupId: string;
  targetTime: string;
  targetLsn: string;
  targetNode: string;
  status: string;
  createdAt: string;
  completedAt: string;
}

interface BackupSchedule {
  id: string;
  clusterId: string;
  cron: string;
  type: string;
  retentionCount: number;
  retentionDays: number;
  storageConfigId: string;
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

interface Pagination {
  page: number;
  pageSize: number;
  total: number;
}

export function useBackups(clusterId?: string, page = 1, pageSize = 20) {
  return useQuery({
    queryKey: ["backups", clusterId, page, pageSize],
    queryFn: () =>
      api.post<{ backups: Backup[]; pagination: Pagination }>(
        "/skylex.v1.BackupService/ListBackups",
        { clusterId: clusterId || "", page, pageSize },
      ),
  });
}

export function useCreateBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { clusterId: string; type?: string }) =>
      api.post<{ backup: Backup }>("/skylex.v1.BackupService/CreateBackup", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["backups"] });
    },
  });
}

export function useDeleteBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      api.post<{}>("/skylex.v1.BackupService/DeleteBackup", { id }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["backups"] });
    },
  });
}

export function useSchedules(clusterId?: string) {
  return useQuery({
    queryKey: ["schedules", clusterId],
    queryFn: () =>
      api.post<{ schedules: BackupSchedule[] }>(
        "/skylex.v1.ScheduleService/ListSchedules",
        { clusterId: clusterId || "" },
      ),
  });
}

export function useCreateRestoreJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: {
      clusterId: string;
      backupId: string;
      targetTime?: string;
      targetLsn?: string;
      targetNode?: string;
    }) => api.post<{ restoreJob: RestoreJob }>(
      "/skylex.v1.BackupService/CreateRestoreJob",
      input,
    ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["restore"] });
    },
  });
}

export type { Backup, RestoreJob, BackupSchedule };