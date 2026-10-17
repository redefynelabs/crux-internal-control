import { redirect } from "next/navigation";
import { NextResponse } from "next/server";
import { sessionToken } from "@/lib/api";

// The signed file, downloaded through the admin app so the session cookie, not a token in a link, authorizes it.
export async function GET(_: Request, { params }: { params: Promise<{ id: string; version: string }> }) {
  const token = await sessionToken();
  if (!token) redirect("/login");
  const { id, version } = await params;
  const base = (process.env.CONTROL_PLANE_URL ?? "http://localhost:8090").replace(/\/$/, "");
  const res = await fetch(`${base}/v1/installations/${encodeURIComponent(id)}/entitlements/${encodeURIComponent(version)}/file`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  if (!res.ok) return new NextResponse("Not found", { status: res.status === 401 ? 401 : 404 });
  return new NextResponse(res.body, {
    headers: {
      "Content-Type": "application/json",
      "Content-Disposition": res.headers.get("Content-Disposition") ?? 'attachment; filename="entitlement.json"',
      "Cache-Control": "no-store",
    },
  });
}
