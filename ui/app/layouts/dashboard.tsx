import { useEffect, useRef } from "react";
import { NavLink, Outlet, useNavigate, useLocation } from "react-router";
import { useAuth } from "~/lib/auth";
import { Button } from "~/components/ui/button";
import TopNavbar from "~/components/TopNavbar";
import { useToast } from "~/components/ui/toast";
import { useNodes } from "~/hooks/useNodes";
import {
  LayoutDashboard,
  Server,
  Network,
  History,
  HardDrive,
  Settings,
  FileText,
  LogOut,
} from "lucide-react";

const navItems = [
  { to: "/", label: "Dashboard", icon: LayoutDashboard },
  { to: "/clusters", label: "Clusters", icon: Server },
  { to: "/nodes", label: "Nodes", icon: Network },
  { to: "/backups", label: "Backups", icon: History },
  { to: "/storage", label: "Storage", icon: HardDrive },
  { to: "/settings", label: "Settings", icon: Settings },
  { to: "/audit", label: "Audit Logs", icon: FileText },
];

export default function DashboardLayout() {
  const { user, isAuthenticated, isLoading, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const { data: nodesData } = useNodes(undefined, 1, 100, isAuthenticated);
  const { success } = useToast();
  const prevNodeIds = useRef<Set<string> | null>(null);

  useEffect(() => {
    if (!isLoading && !isAuthenticated) {
      navigate("/login", { replace: true });
    }
  }, [isLoading, isAuthenticated, navigate]);

  useEffect(() => {
    if (!isAuthenticated || !nodesData?.nodes) return;
    const currentNodes = nodesData.nodes;

    if (prevNodeIds.current === null) {
      prevNodeIds.current = new Set(currentNodes.map((n) => n.id));
      return;
    }

    for (const node of currentNodes) {
      if (!prevNodeIds.current.has(node.id)) {
        const nodeAddress = node.address || node.hostname;
        success(
          "New Node Registered",
          `Node '${node.hostname}' (${nodeAddress}:${node.port}) has registered successfully.`
        );
        prevNodeIds.current.add(node.id);
      }
    }

    const currentIds = new Set(currentNodes.map((n) => n.id));
    for (const id of prevNodeIds.current) {
      if (!currentIds.has(id)) {
        prevNodeIds.current.delete(id);
      }
    }
  }, [nodesData, isAuthenticated, success]);

  if (isLoading || !isAuthenticated) {
    return null;
  }

  const isClusterDetail =
    location.pathname.startsWith("/clusters/") &&
    location.pathname !== "/clusters/create" &&
    location.pathname !== "/clusters";

  return (
    <div className="flex flex-col h-screen bg-background text-foreground overflow-hidden">
      <TopNavbar />
      
      <div className="flex flex-1 overflow-hidden h-[calc(100vh-3.5rem)]">
        {!isClusterDetail && (
          <aside className="w-60 bg-sidebar text-sidebar-foreground border-r border-sidebar-border flex flex-col h-full shrink-0">
            <div className="px-6 pt-5 pb-2">
              <span className="text-[10px] font-bold tracking-wider text-muted-foreground uppercase">Project</span>
            </div>
            <nav className="flex-1 px-3 py-2 space-y-1 overflow-y-auto">
              {navItems.map((item) => {
                const Icon = item.icon;
                return (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    end={item.to === "/"}
                    className={({ isActive }) =>
                      `flex items-center gap-2.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                        isActive
                          ? "bg-sidebar-accent text-sidebar-accent-foreground font-semibold"
                          : "text-sidebar-foreground/75 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
                      }`
                    }
                  >
                    <Icon className="size-4 shrink-0" />
                    {item.label}
                  </NavLink>
                );
              })}
            </nav>
            <div className="p-4 border-t border-sidebar-border space-y-2 bg-muted/20">
              {user && (
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs text-sidebar-foreground/60 truncate font-semibold" title={user.email}>
                    {user.email}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    onClick={logout}
                    title="Logout"
                    className="text-muted-foreground hover:text-destructive hover:bg-destructive/10 shrink-0 cursor-pointer"
                  >
                    <LogOut className="size-3.5" />
                  </Button>
                </div>
              )}
              <div className="text-[10px] text-sidebar-foreground/40 font-medium">Skylex v0.1.0</div>
            </div>
          </aside>
        )}
        <main className={`flex-1 bg-background ${isClusterDetail ? "flex flex-col overflow-hidden h-full" : "overflow-auto h-full"}`}>
          {isClusterDetail ? (
            <Outlet />
          ) : (
            <div className="p-8 max-w-7xl mx-auto">
              <Outlet />
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

