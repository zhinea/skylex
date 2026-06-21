import { useState, useRef, useEffect } from "react";
import { useParams, useNavigate, useLocation, Link } from "react-router";
import { useAuth } from "~/lib/auth";
import { useClusters } from "~/hooks/useClusters";
import { Button } from "~/components/ui/button";
import {
  ChevronDown,
  Sparkles,
  HelpCircle,
  ExternalLink,
  LogOut,
  User,
  Server,
  CheckCircle2,
  AlertTriangle,
  Send,
  X,
  Database,
  Terminal,
  FileText,
  Activity,
  Plus
} from "lucide-react";

export default function TopNavbar() {
  const { user, logout } = useAuth();
  const { id: activeClusterId } = useParams<{ id?: string }>();
  const navigate = useNavigate();
  const location = useLocation();

  const [teamOpen, setTeamOpen] = useState(false);
  const [clusterOpen, setClusterOpen] = useState(false);
  const [profileOpen, setProfileOpen] = useState(false);
  const [aiOpen, setAiOpen] = useState(false);

  const teamRef = useRef<HTMLDivElement>(null);
  const clusterRef = useRef<HTMLDivElement>(null);
  const profileRef = useRef<HTMLDivElement>(null);

  // Fetch all clusters for the selector
  const { data: clustersData } = useClusters(1, 50);
  const clusters = clustersData?.clusters || [];

  // Find currently active cluster
  const activeCluster = clusters.find((c) => c.id === activeClusterId);

  // Close dropdowns on click outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      const target = event.target as Node;
      if (teamRef.current && !teamRef.current.contains(target)) {
        setTeamOpen(false);
      }
      if (clusterRef.current && !clusterRef.current.contains(target)) {
        setClusterOpen(false);
      }
      if (profileRef.current && !profileRef.current.contains(target)) {
        setProfileOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Compute initials for profile picture
  const userInitials = (() => {
    if (!user) return "U";
    if (user.displayName) {
      const parts = user.displayName.split(" ");
      return parts.map((p) => p[0]).join("").toUpperCase().slice(0, 2);
    }
    return user.email.slice(0, 2).toUpperCase();
  })();

  // Compute team/personal label
  const teamLabel = user?.displayName ? `${user.displayName}` : user?.email ? user.email.split("@")[0] : "Personal Team";

  // Compute system health (if any cluster has issues or nodes are offline)
  const isHealthy = clusters.length === 0 || clusters.every((c) => c.status === "RUNNING" || c.status === "HEALTHY");

  // AI Assistant Chat state
  const [messages, setMessages] = useState<Array<{ sender: "ai" | "user"; text: string; time: string }>>([
    {
      sender: "ai",
      text: "Hi! I am the Skylex AI Copilot. I can help you write PostgreSQL queries, check replication lag, diagnose agent issues, or draft auto-scaling rules. What are we building today?",
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    }
  ]);
  const [inputVal, setInputVal] = useState("");
  const chatEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages]);

  const handleSendMessage = () => {
    if (!inputVal.trim()) return;
    const userMsg = inputVal.trim();
    const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    setMessages((prev) => [...prev, { sender: "user", text: userMsg, time: now }]);
    setInputVal("");

    // Simple reactive simulated AI responses based on keywords
    setTimeout(() => {
      let reply = "I'm analyzing the control plane metrics. Would you like me to check the active queries or run a preflight installer command?";
      const lower = userMsg.toLowerCase();
      if (lower.includes("replication") || lower.includes("lag") || lower.includes("primary") || lower.includes("replica")) {
        reply = "Looking at your Postgres setup: you are using asynchronous stream replication. Both primary and replica nodes are connected. Health status is excellent with negligible latency.";
      } else if (lower.includes("install") || lower.includes("docker") || lower.includes("agent")) {
        reply = "To register a new DB node, run our provisioning command on the host. Under 'Nodes', click 'Install Agent' to copy the registration token and docker run scripts.";
      } else if (lower.includes("backup") || lower.includes("restore") || lower.includes("storage")) {
        reply = "You can define S3-compatible storage buckets under the Storage tab. Once configured, automated Point-in-Time Recovery (PITR) and WAL archiving can be scheduled.";
      } else if (lower.includes("query") || lower.includes("sql") || lower.includes("select")) {
        reply = "Here's a template to inspect active lock contentions in PostgreSQL:\n\n```sql\nSELECT pid, age(clock_timestamp(), query_start), usename, query\nFROM pg_stat_activity\nWHERE state != 'idle'\nORDER BY 2 DESC;\n```";
      }

      setMessages((prev) => [...prev, { sender: "ai", text: reply, time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) }]);
    }, 1000);
  };

  return (
    <>
      <header className="h-14 border-b border-sidebar-border bg-sidebar flex items-center justify-between px-6 select-none z-40 shrink-0">
        {/* Left Side: Brand Logo, Divider, Team Dropdown, Divider, Cluster Dropdown */}
        <div className="flex items-center gap-1.5 h-full">
          {/* Brand Logo & Name */}
          <Link to="/" className="flex items-center gap-2 hover:opacity-95 transition-opacity">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" className="text-emerald-500">
              <rect x="3" y="3" width="7" height="7" rx="1.5" className="fill-emerald-500/90" />
              <rect x="14" y="3" width="7" height="7" rx="1.5" className="fill-emerald-500/60" />
              <rect x="3" y="14" width="7" height="7" rx="1.5" className="fill-emerald-500/60" />
              <rect x="14" y="14" width="7" height="7" rx="1.5" className="fill-emerald-500/20" />
            </svg>
            <span className="font-extrabold tracking-tight text-base text-foreground bg-gradient-to-r from-foreground to-foreground/80 bg-clip-text">
              Skylex
            </span>
          </Link>

          <span className="text-muted-foreground/30 font-light text-base px-2">/</span>

          {/* Team Dropdown */}
          <div className="relative" ref={teamRef}>
            <button
              onClick={() => setTeamOpen(!teamOpen)}
              className="flex items-center gap-2 px-2.5 py-1.5 rounded-md hover:bg-sidebar-accent/50 text-sm font-semibold text-foreground transition-all cursor-pointer"
            >
              <span className="truncate max-w-[120px]">{teamLabel}</span>
              <span className="text-[10px] font-bold px-1.5 py-0.5 rounded-sm bg-muted text-muted-foreground/90 border border-border/60">
                Self-hosted
              </span>
              <ChevronDown className="size-3 text-muted-foreground/60 transition-transform duration-200" style={{ transform: teamOpen ? 'rotate(180deg)' : 'none' }} />
            </button>

            {teamOpen && (
              <div className="absolute left-0 mt-1.5 w-52 rounded-lg border border-border bg-popover text-popover-foreground shadow-lg p-1.5 z-50 animate-in fade-in slide-in-from-top-1 duration-200">
                <div className="px-2 py-1.5 text-[10px] uppercase font-bold tracking-wider text-muted-foreground">
                  Control Plane Access
                </div>
                <div className="flex items-center gap-2.5 p-2 rounded-md bg-muted/40 border border-border/40">
                  <div className="size-7 rounded-full bg-emerald-500/10 border border-emerald-500/25 flex items-center justify-center font-bold text-xs text-emerald-500">
                    {userInitials}
                  </div>
                  <div className="overflow-hidden">
                    <p className="text-xs font-bold truncate">{user?.displayName || "Default User"}</p>
                    <p className="text-[10px] text-muted-foreground truncate">{user?.email}</p>
                  </div>
                </div>
                <div className="h-px bg-border/60 my-1.5" />
                <button
                  onClick={() => { setTeamOpen(false); navigate("/settings"); }}
                  className="w-full flex items-center gap-2 px-2 py-1.5 text-xs text-left rounded-md hover:bg-muted transition-colors cursor-pointer"
                >
                  <User className="size-3.5 text-muted-foreground" />
                  <span>Access Management</span>
                </button>
              </div>
            )}
          </div>

          {activeClusterId && (
            <>
              <span className="text-muted-foreground/30 font-light text-base px-2">/</span>

              {/* Cluster Dropdown Selector */}
              <div className="relative" ref={clusterRef}>
                <button
                  onClick={() => setClusterOpen(!clusterOpen)}
                  className="flex items-center gap-2 px-2.5 py-1.5 rounded-md hover:bg-sidebar-accent/50 text-sm font-semibold text-foreground transition-all cursor-pointer"
                >
                  <Database className="size-3.5 text-emerald-500/80" />
                  <span className="truncate max-w-[160px]">
                    {activeCluster ? activeCluster.name : "Select Cluster"}
                  </span>
                  <ChevronDown className="size-3 text-muted-foreground/60 transition-transform duration-200" style={{ transform: clusterOpen ? 'rotate(180deg)' : 'none' }} />
                </button>

                {clusterOpen && (
                  <div className="absolute left-0 mt-1.5 w-64 rounded-lg border border-border bg-popover text-popover-foreground shadow-lg p-1.5 z-50 animate-in fade-in slide-in-from-top-1 duration-200">
                    <div className="px-2 py-1.5 text-[10px] uppercase font-bold tracking-wider text-muted-foreground flex items-center justify-between">
                      <span>Clusters ({clusters.length})</span>
                      <Link to="/clusters/create" onClick={() => setClusterOpen(false)} className="text-emerald-500 hover:text-emerald-400 flex items-center gap-0.5">
                        <Plus className="size-3" /> New
                      </Link>
                    </div>
                    <div className="max-h-60 overflow-y-auto space-y-0.5">
                      {clusters.length === 0 ? (
                        <p className="text-[11px] text-muted-foreground p-3 text-center">No active clusters</p>
                      ) : (
                        clusters.map((c) => (
                          <button
                            key={c.id}
                            onClick={() => {
                              setClusterOpen(false);
                              navigate(`/clusters/${c.id}`);
                            }}
                            className={`w-full flex items-center justify-between px-2.5 py-2 text-sm rounded-md text-left transition-colors cursor-pointer ${
                              activeClusterId === c.id
                                ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold"
                                : "hover:bg-muted text-foreground/90 border border-transparent"
                            }`}
                          >
                            <div className="flex items-center gap-2 overflow-hidden">
                              <Server className={`size-3.5 shrink-0 ${activeClusterId === c.id ? "text-emerald-400" : "text-muted-foreground"}`} />
                              <span className="truncate">{c.name}</span>
                            </div>
                            <span className={`text-[8px] font-bold px-1.5 py-0.5 rounded-full uppercase border shrink-0 ${
                              c.status === "RUNNING" || c.status === "HEALTHY"
                                ? "bg-emerald-500/10 border-emerald-500/20 text-emerald-400"
                                : "bg-amber-500/10 border-amber-500/20 text-amber-400"
                            }`}>
                              {c.status}
                            </span>
                          </button>
                        ))
                      )}
                    </div>
                  </div>
                )}
              </div>
            </>
          )}
        </div>

        {/* Right Side: Health Status, Ask AI button, Help, Docs, User Dropdown */}
        <div className="flex items-center gap-4">
          {/* Health Status Pill */}
          <div className="hidden sm:flex items-center gap-1.5 px-2.5 py-1 rounded-full bg-muted/30 border border-border/40 text-xs font-medium text-muted-foreground">
            <span className="relative flex h-1.5 w-1.5">
              <span className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-75 ${isHealthy ? 'bg-emerald-400' : 'bg-amber-400'}`}></span>
              <span className={`relative inline-flex rounded-full h-1.5 w-1.5 ${isHealthy ? 'bg-emerald-500' : 'bg-amber-500'}`}></span>
            </span>
            <span>{isHealthy ? "All OK" : "System Warning"}</span>
          </div>

          {/* Ask AI Action Button */}
          <button
            onClick={() => setAiOpen(true)}
            className="relative inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-semibold text-emerald-500 hover:text-emerald-400 bg-emerald-500/5 hover:bg-emerald-500/10 border border-emerald-500/20 transition-all cursor-pointer overflow-hidden shadow-[0_0_8px_rgba(16,185,129,0.05)] hover:shadow-[0_0_12px_rgba(16,185,129,0.15)] group"
          >
            <Sparkles className="size-3.5 fill-emerald-500/10 group-hover:scale-110 transition-transform duration-200" />
            <span>Ask AI</span>
          </button>

          {/* Documentation Link */}
          <a
            href="https://github.com/zhinea/skylex"
            target="_blank"
            rel="noreferrer"
            className="p-1.5 text-muted-foreground hover:text-foreground rounded-md transition-colors"
            title="Read documentation"
          >
            <HelpCircle className="size-4" />
          </a>

          {/* Profile Dropdown */}
          <div className="relative" ref={profileRef}>
            <button
              onClick={() => setProfileOpen(!profileOpen)}
              className="flex items-center gap-1 rounded-full focus:outline-none cursor-pointer focus:ring-2 focus:ring-emerald-500/20"
            >
              <div className="size-8 rounded-full bg-gradient-to-tr from-emerald-600 to-teal-400 border border-emerald-500/20 flex items-center justify-center font-bold text-xs text-white shadow-sm">
                {userInitials}
              </div>
            </button>

            {profileOpen && (
              <div className="absolute right-0 mt-1.5 w-56 rounded-lg border border-border bg-popover text-popover-foreground shadow-lg p-1.5 z-50 animate-in fade-in slide-in-from-top-1 duration-200">
                <div className="px-2.5 py-2">
                  <p className="text-xs font-semibold text-foreground truncate">{user?.displayName || "Skylex Operator"}</p>
                  <p className="text-[10px] text-muted-foreground truncate">{user?.email}</p>
                </div>
                <div className="h-px bg-border/60 my-1" />
                <button
                  onClick={() => { setProfileOpen(false); navigate("/settings"); }}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-xs text-left rounded-md hover:bg-muted transition-colors cursor-pointer"
                >
                  <User className="size-3.5 text-muted-foreground" />
                  Settings
                </button>
                <div className="h-px bg-border/60 my-1" />
                <button
                  onClick={() => {
                    setProfileOpen(false);
                    logout();
                  }}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-xs text-left rounded-md hover:bg-destructive/10 text-destructive font-medium transition-colors cursor-pointer"
                >
                  <LogOut className="size-3.5" />
                  Sign Out
                </button>
              </div>
            )}
          </div>
        </div>
      </header>

      {/* AI Assistant Right Drawer */}
      {aiOpen && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-xs z-50 flex justify-end animate-in fade-in duration-200">
          <div className="w-full max-w-md bg-sidebar border-l border-sidebar-border h-full flex flex-col shadow-2xl relative animate-in slide-in-from-right duration-300">
            {/* Drawer Header */}
            <div className="h-14 border-b border-sidebar-border px-5 flex items-center justify-between shrink-0 bg-sidebar/80 backdrop-blur-md">
              <div className="flex items-center gap-2">
                <div className="size-7 rounded-lg bg-emerald-500/10 flex items-center justify-center border border-emerald-500/20">
                  <Sparkles className="size-4 text-emerald-500" />
                </div>
                <div>
                  <h3 className="text-sm font-bold text-foreground">Skylex AI Copilot</h3>
                  <p className="text-[10px] text-emerald-500 font-medium">Online & Ready</p>
                </div>
              </div>
              <button
                onClick={() => setAiOpen(false)}
                className="p-1.5 rounded-md hover:bg-muted text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              >
                <X className="size-4" />
              </button>
            </div>

            {/* Chat Messages */}
            <div className="flex-1 overflow-y-auto p-5 space-y-4">
              {messages.map((m, idx) => (
                <div key={idx} className={`flex flex-col ${m.sender === "user" ? "items-end" : "items-start"}`}>
                  <div className={`max-w-[85%] rounded-2xl px-4 py-3 text-xs leading-relaxed ${
                    m.sender === "user"
                      ? "bg-emerald-600 text-white rounded-br-none"
                      : "bg-muted text-foreground border border-border/40 rounded-bl-none"
                  }`}>
                    {m.text.includes("```") ? (
                      <div className="space-y-2">
                        <p>{m.text.split("```")[0]}</p>
                        <pre className="p-2.5 rounded bg-zinc-950 text-zinc-200 font-mono text-[10px] overflow-x-auto border border-zinc-800">
                          <code>{m.text.split("```")[1]?.replace("sql\n", "").replace("\n```", "")}</code>
                        </pre>
                      </div>
                    ) : (
                      m.text
                    )}
                  </div>
                  <span className="text-[9px] text-muted-foreground mt-1 px-1">{m.time}</span>
                </div>
              ))}
              <div ref={chatEndRef} />
            </div>

            {/* Suggested Prompts */}
            <div className="px-5 py-2 border-t border-sidebar-border/40 bg-muted/20 flex flex-wrap gap-1.5 shrink-0">
              <button
                onClick={() => setInputVal("Explain replication status")}
                className="px-2.5 py-1 rounded bg-muted/60 hover:bg-muted border border-border/40 text-[10px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              >
                Replication Status
              </button>
              <button
                onClick={() => setInputVal("How to register a new DB node?")}
                className="px-2.5 py-1 rounded bg-muted/60 hover:bg-muted border border-border/40 text-[10px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              >
                Add Node
              </button>
              <button
                onClick={() => setInputVal("Show query to check active lock contentions")}
                className="px-2.5 py-1 rounded bg-muted/60 hover:bg-muted border border-border/40 text-[10px] text-muted-foreground hover:text-foreground transition-colors cursor-pointer"
              >
                Query Lock Contentions
              </button>
            </div>

            {/* Chat Input */}
            <div className="p-4 border-t border-sidebar-border bg-sidebar/80 backdrop-blur-md shrink-0">
              <div className="relative flex items-center">
                <input
                  type="text"
                  value={inputVal}
                  onChange={(e) => setInputVal(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && handleSendMessage()}
                  placeholder="Ask a question..."
                  className="w-full pl-3 pr-10 py-2.5 text-xs rounded-lg bg-muted border border-border/60 focus:border-emerald-500/50 focus:ring-1 focus:ring-emerald-500/50 text-foreground transition-all outline-none"
                />
                <button
                  onClick={handleSendMessage}
                  className="absolute right-2 p-1.5 rounded-md bg-emerald-600 hover:bg-emerald-500 text-white transition-colors cursor-pointer"
                >
                  <Send className="size-3" />
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
