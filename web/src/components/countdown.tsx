"use client";

import { useEffect, useState } from "react";

/** Time left until an instant, ticking every 15 seconds. Rendered empty until mounted so the server and browser agree. */
export function Countdown({ until }: { until: string }) {
  const [now, setNow] = useState<number | null>(null);
  useEffect(() => {
    const tick = () => setNow(Date.now());
    tick();
    const id = setInterval(tick, 15_000);
    return () => clearInterval(id);
  }, []);
  if (now === null) return <span className="tabular-nums text-muted-foreground">…</span>;
  const secs = Math.round((new Date(until).getTime() - now) / 1000);
  if (secs <= 0) return <span className="text-muted-foreground">ending</span>;
  const h = Math.floor(secs / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return <span className="tabular-nums">{h > 0 ? `${h}h ${m}m` : `${Math.max(m, 1)}m`} left</span>;
}
