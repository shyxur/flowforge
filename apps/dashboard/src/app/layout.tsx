import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { SidebarNav } from "@/components/sidebar-nav";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "windylane dashboard",
    template: "%s · windylane",
  },
  description: "operations dashboard for windylane.",
  icons: {
    icon: "/brand/windylane-mark.png",
  },
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
              <span className="brand-logo-frame">
                <Image
                  alt="windylane"
                  className="brand-logo"
                  height={1024}
                  priority
                  src="/brand/windylane-logo.png"
                  width={1536}
                />
              </span>
              <small>stay in flow.</small>
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
                <span>windylane control plane</span>
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
