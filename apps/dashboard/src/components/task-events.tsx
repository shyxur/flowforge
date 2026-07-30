"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

export function TaskEvents() {
  const router = useRouter();

  useEffect(() => {
    const events = new EventSource("/api/events/tasks");
    let refreshTimer: ReturnType<typeof setTimeout> | undefined;
    const refresh = () => {
      clearTimeout(refreshTimer);
      refreshTimer = setTimeout(() => router.refresh(), 150);
    };
    events.addEventListener("task", refresh);
    return () => {
      clearTimeout(refreshTimer);
      events.removeEventListener("task", refresh);
      events.close();
    };
  }, [router]);

  return null;
}
