import { NextRequest, NextResponse } from "next/server";
import { SCENARIOS_DATA } from "@/lib/scenarioData";

export async function GET(req: NextRequest) {
  const goApiUrl = process.env.HISAAB_API_URL || process.env.NEXT_PUBLIC_API_URL || "http://127.0.0.1:8080";
  const callerRole = req.headers.get("X-Hisaab-Role") || "COLLECTOR_RSP";

  try {
    const res = await fetch(`${goApiUrl}/v1/scenarios`, {
      headers: {
        "X-Hisaab-Role": callerRole,
      },
      signal: AbortSignal.timeout(2000),
    });

    if (res.ok) {
      const data = await res.json();
      return NextResponse.json({
        source: "live_go_engine",
        count: data.count || (data.scenarios ? data.scenarios.length : 0),
        scenarios: data.scenarios || [],
      });
    }
  } catch (_e) {
    // Graceful fallback to static scenario data when Go backend is offline
  }

  // Fallback to scenario catalog
  const catalog = SCENARIOS_DATA.map((s) => ({
    id: s.id,
    title: s.title,
    brd_section: s.brdSection,
    category: s.category,
    status: s.status,
    np_pair: s.npPair,
    orders_count: s.ordersCount,
    gross_paise: s.grossPaise,
    net_payout_paise: s.netPayoutPaise,
  }));

  return NextResponse.json({
    source: "catalog_fallback",
    count: catalog.length,
    scenarios: catalog,
  });
}
