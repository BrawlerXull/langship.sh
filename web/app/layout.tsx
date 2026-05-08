import type { Metadata } from "next";
import "./globals.css";

import { AppSidebar } from "@/components/app-sidebar";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";

export const metadata: Metadata = {
  title: "flow",
  description: "Durable n8n-compatible workflow engine",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" suppressHydrationWarning>
      <body className="min-h-screen bg-background font-sans antialiased">
        <SidebarProvider
          style={{ "--sidebar-width": "17rem" } as React.CSSProperties}
        >
          <AppSidebar />
          <SidebarInset>
            <div className="flex min-h-0 flex-1 flex-col">{children}</div>
          </SidebarInset>
        </SidebarProvider>
      </body>
    </html>
  );
}
