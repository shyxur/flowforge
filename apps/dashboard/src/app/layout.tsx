import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "QueueFlow Dashboard",
    template: "%s · QueueFlow",
  },
  description: "Operations dashboard for FlowForge QueueFlow.",
};

const navigation = [
  { href: "/", label: "Overview", mark: "O" },
  { href: "/tasks", label: "Tasks", mark: "T" },
  { href: "/workers", label: "Workers", mark: "W" },
  { href: "/dlq", label: "Dead letter", mark: "D" },
  { href: "/webhooks", label: "Webhooks", mark: "H" },
  { href: "/webhook-deliveries", label: "Webhook deliveries", mark: "L" },
];

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <div className="app-shell">
          <aside className="sidebar">
            <Link className="brand" href="/">
              <span className="brand-mark">Q</span>
              <span>
                <strong>QueueFlow</strong>
                <small>FlowForge</small>
              </span>
            </Link>

            <nav aria-label="Primary navigation" className="nav-list">
              {navigation.map((item) => (
                <Link href={item.href} key={item.href}>
                  <span className="nav-mark">{item.mark}</span>
                  {item.label}
                </Link>
              ))}
            </nav>

            <div className="sidebar-foot">
              <span className="status-dot" />
              <span>
                <strong>Local environment</strong>
                <small>API connection pending</small>
              </span>
            </div>
          </aside>

          <div className="main-column">
            <header className="topbar">
              <div>
                <p>Queue operations</p>
                <span>Development organization</span>
              </div>
              <div className="topbar-actions">
                <span className="environment-badge">DEV</span>
                <span className="avatar">DF</span>
              </div>
            </header>
            <main className="content">{children}</main>
          </div>
        </div>
      </body>
    </html>
  );
}
