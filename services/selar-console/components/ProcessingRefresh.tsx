"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

export function ProcessingRefresh({ active }: { active: boolean }) {
  const router = useRouter();

  useEffect(() => {
    if (!active) return;
    let refreshes = 0;
    const interval = window.setInterval(() => {
      if (refreshes >= 60) {
        window.clearInterval(interval);
        return;
      }
      refreshes += 1;
      router.refresh();
    }, 5000);
    return () => window.clearInterval(interval);
  }, [active, router]);

  return null;
}
