import { NextRequest, NextResponse } from "next/server";

export async function GET(req: NextRequest) {
  const goApiUrl = process.env.HISAAB_API_URL || process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8080";
  const callerRole = req.headers.get("X-Hisaab-Role") || "AUDITOR";

  try {
    const res = await fetch(`${goApiUrl}/v1/worker/telemetry`, {
      headers: {
        "X-Hisaab-Role": callerRole,
      },
      signal: AbortSignal.timeout(2000),
    });

    if (res.ok) {
      const data = await res.json();
      return NextResponse.json({
        source: "live_go_worker",
        ...data,
      });
    }
  } catch (_e) {
    // Fallback if offline
  }

  // Simulated queue telemetry when Go engine is offline
  return NextResponse.json({
    source: "offline_simulation",
    uptime_seconds: 3600,
    database_connected: false,
    pending_outbox_events: 0,
    dispatched_outbox_events: 142,
    failed_outbox_events: 0,
    total_batches_cached: 22,
    timestamp: new Date().toISOString(),
    warning: "Worker service offline; showing cached queue baseline",
  });
}
