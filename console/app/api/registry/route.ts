import { NextRequest, NextResponse } from "next/server";

export async function GET(req: NextRequest) {
  const { searchParams } = new URL(req.url);
  const subscriberId = searchParams.get("subscriber_id") || searchParams.get("subscriberId");
  const uniqueKeyId = searchParams.get("unique_key_id") || searchParams.get("uniqueKeyId") || "key-1";

  if (!subscriberId) {
    return NextResponse.json({ error: "subscriber_id query param is required" }, { status: 400 });
  }

  const goApiUrl = process.env.HISAAB_API_URL || process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8080";
  const callerRole = req.headers.get("X-Hisaab-Role") || "COLLECTOR_RSP";

  try {
    const res = await fetch(`${goApiUrl}/v1/registry/lookup?subscriber_id=${encodeURIComponent(subscriberId)}&unique_key_id=${encodeURIComponent(uniqueKeyId)}`, {
      headers: {
        "X-Hisaab-Role": callerRole,
      },
      signal: AbortSignal.timeout(2000),
    });

    if (res.ok) {
      const data = await res.json();
      return NextResponse.json({
        source: "live_go_registry",
        subscriberId,
        uniqueKeyId,
        publicKey: data.public_key,
        verified: data.verified,
      });
    }
  } catch (_e) {
    // Fallback if offline
  }

  // Simulated fallback key for offline workbench development
  return NextResponse.json({
    source: "offline_mock",
    subscriberId,
    uniqueKeyId,
    publicKey: "MCowBQYDK2VwAyEA9b45620958172c3d8e5f2a1b4c3d2e1f0a9b8c7d6e5=",
    verified: false,
    warning: "ONDC Registry unreachable; using cached simulator key",
  });
}
