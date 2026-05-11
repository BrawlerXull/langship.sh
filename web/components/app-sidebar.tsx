"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Bot,
  GitBranch,
  Play,
  CheckSquare,
  Layers,
  Lock,
  Radio,
  PanelLeft,
  PanelLeftClose,
  Sun,
  MoonStar,
  Monitor,
} from "lucide-react";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";

type NavItem = {
  title: string;
  href: string;
  icon: React.ComponentType<{ className?: string }>;
  match?: (pathname: string) => boolean;
};

const primary: NavItem[] = [
  // {
  //   title: "Dashboard",
  //   href: "/dashboard",
  //   icon: LayoutDashboard,
  //   match: (p) => p === "/dashboard",
  // },
  {
    title: "Agents",
    href: "/agents",
    icon: Bot,
    match: (p) => p.startsWith("/agents"),
  },

  {
    title: "Environments",
    href: "/environments",
    icon: Layers,
    match: (p) => p.startsWith("/environments"),
  },
  {
    title: "Pipelines",
    href: "/",
    icon: GitBranch,
    match: (p) => p === "/" || p.startsWith("/flows"),
  },
  {
    title: "Runs",
    href: "/runs",
    icon: Play,
    match: (p) => p === "/runs" || p.startsWith("/executions"),
  },
  {
    title: "Approvals",
    href: "/approvals",
    icon: CheckSquare,
    match: (p) => p.startsWith("/approvals"),
  },
 
  {
    title: "Gateway",
    href: "/gateway",
    icon: Radio,
    match: (p) => p.startsWith("/gateway"),
  },
    {
    title: "Credentials",
    href: "/credentials",
    icon: Lock,
    match: (p) => p.startsWith("/credentials"),
  },
];

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const pathname = usePathname() || "/";

  return (
    <Sidebar variant="floating" collapsible="icon" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href="/">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <LangshipMark />
                </div>
                <div className="flex flex-col gap-0.5 leading-none">
                  <span className="font-semibold">Lyzrship.sh</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu className="gap-1">
            {primary.map((item) => {
              const Icon = item.icon;
              const active = item.match
                ? item.match(pathname)
                : pathname === item.href;
              return (
                <SidebarMenuItem key={item.href}>
                  <SidebarMenuButton asChild isActive={active}>
                    <Link href={item.href} className="font-medium">
                      <Icon className="size-4" />
                      <span>{item.title}</span>
                    </Link>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              );
            })}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <CollapseToggle />
          </SidebarMenuItem>
          <SidebarMenuItem>
            <ThemeToggle />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
  );
}

function LangshipMark() {
  // Tiny anchor/route mark — placeholder for the real Langship logo.
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-4"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <circle cx="6" cy="6" r="2" />
      <circle cx="18" cy="18" r="2" />
      <path d="M6 8v6a4 4 0 0 0 4 4h6" />
    </svg>
  );
}

function CollapseToggle() {
  const { toggleSidebar, state } = useSidebar();
  const collapsed = state === "collapsed";
  const Icon = collapsed ? PanelLeft : PanelLeftClose;
  return (
    <SidebarMenuButton onClick={toggleSidebar} className="text-muted-foreground">
      <Icon className="size-4" />
      <span>{collapsed ? "Expand" : "Collapse"}</span>
    </SidebarMenuButton>
  );
}

// ThemeToggle is a 3-way segmented control (Light / System / Dark) that
// writes the theme to <html class>. Persists in localStorage. When the
// sidebar is collapsed to icons, it renders a single button that cycles
// through the three themes instead.
function ThemeToggle() {
  const { state } = useSidebar();
  const collapsed = state === "collapsed";
  const [theme, setTheme] = React.useState<"light" | "dark" | "system">("system");

  React.useEffect(() => {
    const saved =
      (typeof window !== "undefined" &&
        (localStorage.getItem("flow-theme") as
          | "light"
          | "dark"
          | "system"
          | null)) ||
      "system";
    setTheme(saved);
    apply(saved);
  }, []);

  function apply(t: "light" | "dark" | "system") {
    const root = document.documentElement;
    const prefersDark =
      window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;
    const dark = t === "dark" || (t === "system" && prefersDark);
    root.classList.toggle("dark", dark);
    localStorage.setItem("flow-theme", t);
  }

  function pick(next: "light" | "dark" | "system") {
    setTheme(next);
    apply(next);
  }

  const items: { id: "light" | "system" | "dark"; icon: typeof Sun; label: string }[] = [
    { id: "light", icon: Sun, label: "Light" },
    { id: "system", icon: Monitor, label: "System" },
    { id: "dark", icon: MoonStar, label: "Dark" },
  ];

  if (collapsed) {
    const current = items.find((i) => i.id === theme) ?? items[1];
    const Icon = current.icon;
    const next = items[(items.findIndex((i) => i.id === theme) + 1) % items.length].id;
    return (
      <SidebarMenuButton
        onClick={() => pick(next)}
        className="text-muted-foreground"
        title={`Theme: ${current.label} (click to cycle)`}
      >
        <Icon className="size-4" />
        <span>{current.label}</span>
      </SidebarMenuButton>
    );
  }

  return (
    <div className="mx-2 mb-1 flex rounded-md border bg-muted/40 p-0.5">
      {items.map((it) => {
        const Icon = it.icon;
        const active = theme === it.id;
        return (
          <button
            key={it.id}
            type="button"
            onClick={() => pick(it.id)}
            aria-pressed={active}
            className={
              "flex flex-1 items-center justify-center gap-1 rounded-sm px-2 py-1 text-[11px] transition-colors " +
              (active
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground")
            }
          >
            <Icon className="size-3.5" />
            <span>{it.label}</span>
          </button>
        );
      })}
    </div>
  );
}
