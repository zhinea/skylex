import { QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Navigate } from "react-router";

import { AuthProvider } from "~/lib/auth";
import { getQueryClient } from "~/lib/query-client";
import { ToastProvider } from "~/components/ui/toast";

import DashboardLayout from "~/layouts/dashboard";
import LoginPage from "~/routes/login";
import DashboardPage from "~/routes/dashboard";
import ClustersPage from "~/routes/clusters";
import CreateClusterPage from "~/routes/clusters.create";
import ClusterDetailLayout from "~/routes/clusters.$id";
import ClusterOverviewPage from "~/routes/clusters.$id.overview";
import ClusterConnectionPage from "~/routes/clusters.$id.connection";
import ClusterDatabasesPage from "~/routes/clusters.$id.databases";
import ClusterRolesPage from "~/routes/clusters.$id.roles";
import ClusterNetworkPage from "~/routes/clusters.$id.network";
import ClusterTLSPage from "~/routes/clusters.$id.tls";
import ClusterExtensionsPage from "~/routes/clusters.$id.extensions";
import ClusterDiagnosticsPage from "~/routes/clusters.$id.diagnostics";
import ClusterSettingsPage from "~/routes/clusters.$id.settings";
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
                <Route path="clusters/:id" element={<ClusterDetailLayout />}>
                  <Route index element={<Navigate to="overview" replace />} />
                  <Route path="overview" element={<ClusterOverviewPage />} />
                  <Route path="connection" element={<ClusterConnectionPage />} />
                  <Route path="databases" element={<ClusterDatabasesPage />} />
                  <Route path="roles" element={<ClusterRolesPage />} />
                  <Route path="network" element={<ClusterNetworkPage />} />
                  <Route path="tls" element={<ClusterTLSPage />} />
                  <Route path="extensions" element={<ClusterExtensionsPage />} />
                  <Route path="diagnostics" element={<ClusterDiagnosticsPage />} />
                  <Route path="settings" element={<ClusterSettingsPage />} />
                </Route>
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
