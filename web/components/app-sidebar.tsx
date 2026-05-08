"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Workflow,
  LayoutGrid,
  PlusCircle,
  Activity,
  BookOpen,
  ExternalLink,
  Github,
  Bot,
} from "lucide-react";

import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarSeparator,
  useSidebar,
} from "@/components/ui/sidebar";
import { PanelLeftClose, PanelLeft } from "lucide-react";

type NavItem = {
  title: string;
  href: string;
  icon: React.ComponentType<{ className?: string }>;
  match?: (pathname: string) => boolean;
};

const primary: NavItem[] = [
  {
    title: "Pipelines",
    href: "/",
    icon: LayoutGrid,
    match: (p) => p === "/" || p.startsWith("/flows/view"),
  },
  {
    title: "New pipeline",
    href: "/flows/new",
    icon: PlusCircle,
    match: (p) => p.startsWith("/flows/new"),
  },
  {
    title: "Executions",
    href: "/executions/view",
    icon: Activity,
    match: (p) => p.startsWith("/executions"),
  },
  {
    title: "Agents",
    href: "/agents",
    icon: Bot,
    match: (p) => p.startsWith("/agents"),
  },
];

const docs = [
  {
    title: "n8n compatibility",
    href: "https://github.com/lyzrai/flow#n8n-compatible",
    external: true,
  },
  {
    title: "Durability model",
    href: "https://github.com/lyzrai/flow#durability",
    external: true,
  },
  {
    title: "Embed as Go library",
    href: "https://github.com/lyzrai/flow#embedding",
    external: true,
  },
];

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  const pathname = usePathname() || "/";

  return (
    <Sidebar variant="floating" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size="lg" asChild>
              <Link href="/">
                <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
                  <Workflow className="size-4" />
                </div>
                <div className="flex flex-col gap-0.5 leading-none">
                  <span className="font-semibold">flow</span>
                  <span className="text-xs text-muted-foreground">
                    pre-v0.1 · durable pipelines
                  </span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Workspace</SidebarGroupLabel>
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

        <SidebarSeparator />

        <SidebarGroup>
          <SidebarGroupLabel>
            <BookOpen className="mr-1 size-3.5" />
            Reference
          </SidebarGroupLabel>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton className="font-medium" disabled>
                Documentation
              </SidebarMenuButton>
              <SidebarMenuSub className="ml-0 border-l-0 px-1.5">
                {docs.map((d) => (
                  <SidebarMenuSubItem key={d.href}>
                    <SidebarMenuSubButton asChild>
                      <a href={d.href} target="_blank" rel="noreferrer">
                        <span className="truncate">{d.title}</span>
                        {d.external && (
                          <ExternalLink className="ml-auto size-3 opacity-60" />
                        )}
                      </a>
                    </SidebarMenuSubButton>
                  </SidebarMenuSubItem>
                ))}
              </SidebarMenuSub>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton asChild>
              <a
                href="https://github.com/lyzrai/flow"
                target="_blank"
                rel="noreferrer"
              >
                <Github className="size-4" />
                <span>GitHub</span>
                <ExternalLink className="ml-auto size-3 opacity-60" />
              </a>
            </SidebarMenuButton>
          </SidebarMenuItem>
          <SidebarMenuItem>
            <CollapseToggle />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarFooter>
    </Sidebar>
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
