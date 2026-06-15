import { type RouteConfig, layout, route, index } from "@react-router/dev/routes";

export default [
  layout("layouts/dashboard.tsx", [
    index("routes/dashboard.tsx"),
    route("clusters", "routes/clusters.tsx"),
    route("clusters/create", "routes/clusters.create.tsx"),
    route("clusters/:id", "routes/clusters.$id.tsx"),
    route("nodes", "routes/nodes.tsx"),
    route("backups", "routes/backups.tsx"),
    route("restore", "routes/restore.tsx"),
    route("storage", "routes/storage.tsx"),
    route("settings", "routes/settings.tsx"),
    route("audit", "routes/audit.tsx"),
  ]),
  route("login", "routes/login.tsx"),
] satisfies RouteConfig;