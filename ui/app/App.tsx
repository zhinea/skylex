import { QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router";

import { AuthProvider } from "~/lib/auth";
import { getQueryClient } from "~/lib/query-client";
import { ToastProvider } from "~/components/ui/toast";

import DashboardLayout from "~/layouts/dashboard";
import LoginPage from "~/routes/login";
import DashboardPage from "~/routes/dashboard";
import ClustersPage from "~/routes/clusters";
import CreateClusterPage from "~/routes/clusters.create";
import ClusterDetailPage from "~/routes/clusters.$id";
import NodesPage from "~/routes/nodes";
import BackupsPage from "~/routes/backups";
import RestorePage from "~/routes/restore";
import StoragePage from "~/routes/storage";
import SettingsPage from "~/routes/settings";
import AuditPage from "~/routes/audit";

export default function App() {
  return (
    <QueryClientProvider client={getQueryClient()}>
      <BrowserRouter basename="/panel">
        <AuthProvider>
          <ToastProvider>
            <Routes>
              <Route path="login" element={<LoginPage />} />
              <Route element={<DashboardLayout />}>
                <Route index element={<DashboardPage />} />
                <Route path="clusters" element={<ClustersPage />} />
                <Route path="clusters/create" element={<CreateClusterPage />} />
                <Route path="clusters/:id" element={<ClusterDetailPage />} />
                <Route path="nodes" element={<NodesPage />} />
                <Route path="backups" element={<BackupsPage />} />
                <Route path="restore" element={<RestorePage />} />
                <Route path="storage" element={<StoragePage />} />
                <Route path="settings" element={<SettingsPage />} />
                <Route path="audit" element={<AuditPage />} />
              </Route>
            </Routes>
          </ToastProvider>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
