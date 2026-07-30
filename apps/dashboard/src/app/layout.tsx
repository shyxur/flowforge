import type { Metadata } from "next";
import Link from "next/link";
import { SidebarNav } from "@/components/sidebar-nav";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "flowforge dashboard",
    template: "%s · flowforge",
  },
  description: "operations dashboard for flowforge.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <div className="app-shell">
          <aside className="sidebar">
            <Link className="brand" href="/">
              <strong>flowforge</strong>
              <small>queueflow control plane</small>
            </Link>

            <SidebarNav />

            <div className="sidebar-foot">
              <span className="status-dot" />
              <span>
                <strong>development</strong>
                <small>tenant-scoped workspace</small>
              </span>
            </div>
          </aside>

          <div className="main-column">
            <header className="topbar">
              <div>
                <p>operations</p>
                <span>flowforge control plane</span>
              </div>
              <div className="topbar-actions">
                <span className="environment-badge">development</span>
              </div>
            </header>
            <main className="content">{children}</main>
          </div>
        </div>
      </body>
    </html>
  );
}
