"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

const navigation = [
  { href: "/", label: "overview" },
  { href: "/tasks", label: "tasks" },
  { href: "/workers", label: "workers" },
  { href: "/dlq", label: "dlq" },
  { href: "/webhooks", label: "webhooks" },
  { href: "/webhook-deliveries", label: "webhook deliveries" },
];

export function SidebarNav() {
  const pathname = usePathname();

  return (
    <nav aria-label="primary navigation" className="nav-list">
      {navigation.map((item, index) => {
        const active =
          item.href === "/"
            ? pathname === item.href
            : pathname.startsWith(item.href);
        return (
          <Link
            aria-current={active ? "page" : undefined}
            href={item.href}
            key={item.href}
          >
            <span aria-hidden="true" className="nav-mark">
              {String(index + 1).padStart(2, "0")}
            </span>
            {item.label}
          </Link>
        );
      })}
    </nav>
  );
}
