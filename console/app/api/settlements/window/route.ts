import { NextRequest, NextResponse } from "next/server";

// Official RBI Statutory Holidays Database (2024–2027) from internal/calendar/calendar.go
const STATUTORY_HOLIDAYS_2026: Record<string, string> = {
  "2026-01-26": "Republic Day",
  "2026-03-03": "Holi",
  "2026-03-21": "Id-ul-Fitr (Ramzan)",
  "2026-04-02": "Mahavir Jayanti",
  "2026-04-03": "Good Friday",
  "2026-04-14": "Dr. Babasaheb Ambedkar Jayanti",
  "2026-05-01": "Maharashtra Day / May Day",
  "2026-05-27": "Bakrid / Eid-ul-Adha",
  "2026-08-15": "Independence Day",
  "2026-08-26": "Milad-un-Nabi",
  "2026-10-02": "Mahatma Gandhi Jayanti",
  "2026-10-20": "Dussehra",
  "2026-11-08": "Diwali (Laxmi Pujan)",
  "2026-11-24": "Guru Nanak Jayanti",
  "2026-12-25": "Christmas",
};

// Check if a date in IST is an Indian bank holiday
function isBankHolidayIST(date: Date): { isHoliday: boolean; reason: string } {
  const dayOfWeek = date.getDay(); // 0 is Sunday, 6 is Saturday
  if (dayOfWeek === 0) {
    return { isHoliday: true, reason: "Sunday (Statutory Banking Closure)" };
  }

  // 2nd and 4th Saturdays
  if (dayOfWeek === 6) {
    const dayOfMonth = date.getDate();
    const saturdayRank = Math.ceil(dayOfMonth / 7);
    if (saturdayRank === 2) {
      return { isHoliday: true, reason: "2nd Saturday (RBI Negotiable Instruments Act)" };
    }
    if (saturdayRank === 4) {
      return { isHoliday: true, reason: "4th Saturday (RBI Negotiable Instruments Act)" };
    }
  }

  const yyyy = date.getFullYear();
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const dd = String(date.getDate()).padStart(2, "0");
  const dateKey = `${yyyy}-${mm}-${dd}`;

  if (STATUTORY_HOLIDAYS_2026[dateKey]) {
    return { isHoliday: true, reason: STATUTORY_HOLIDAYS_2026[dateKey] };
  }

  return { isHoliday: false, reason: "" };
}

// Roll date forward to next operational business day
function getNextBusinessDayIST(startDate: Date): { targetDate: Date; rolloverHolidays: string[] } {
  let curr = new Date(startDate.getTime());
  const rolloverHolidays: string[] = [];

  while (true) {
    const holidayCheck = isBankHolidayIST(curr);
    if (!holidayCheck.isHoliday) {
      break;
    }
    const dateStr = curr.toISOString().split("T")[0];
    rolloverHolidays.push(`${dateStr}: ${holidayCheck.reason}`);
    curr.setDate(curr.getDate() + 1);
  }

  return { targetDate: curr, rolloverHolidays };
}

export async function POST(req: NextRequest) {
  try {
    const body = await req.json();
    const { submissionTime, tPlusDays = 1, amountPaise = 98400, beneficiaryIfsc = "HDFC0000240", beneficiaryAccount = "50200088921432" } = body;

    const baseTime = submissionTime ? new Date(submissionTime) : new Date();

    // 17:00:00 IST Cut-Off Boundary Check
    // Convert to IST offset (UTC+5:30)
    const istTime = new Date(baseTime.getTime() + (5.5 * 3600 * 1000));
    const hours = istTime.getUTCHours();
    const minutes = istTime.getUTCMinutes();
    const isPastCutoff = hours > 17 || (hours === 17 && minutes > 0);

    // Initial eligibility day
    let eligibilityDate = new Date(baseTime.getTime());
    if (isPastCutoff) {
      eligibilityDate.setDate(eligibilityDate.getDate() + 1);
    }

    // Advance T+N business days
    let current = new Date(eligibilityDate.getTime());
    const rolloverLog: string[] = [];
    if (isPastCutoff) {
      rolloverLog.push("Submission received past 17:00 IST RTGS cutoff -> Rolled to T+1 eligibility base.");
    }

    let daysAdded = 0;
    while (daysAdded < tPlusDays) {
      current.setDate(current.getDate() + 1);
      const check = isBankHolidayIST(current);
      if (check.isHoliday) {
        rolloverLog.push(`Skipped non-business day (${current.toISOString().split("T")[0]}): ${check.reason}`);
      } else {
        daysAdded++;
      }
    }

    // Ensure final settlement instant lands on a business day
    const finalCheck = getNextBusinessDayIST(current);
    const finalSettlementDate = finalCheck.targetDate;
    finalCheck.rolloverHolidays.forEach((h) => rolloverLog.push(`Advanced over holiday: ${h}`));

    const wireInstructionPayload = {
      message_id: `MSG-WIRE-${Date.now()}`,
      instruction_id: `INST-${Date.now()}`,
      payment_type: amountPaise >= 20000000 ? "RTGS" : "NEFT",
      settlement_date: finalSettlementDate.toISOString().split("T")[0],
      amount_paise: amountPaise,
      amount_inr: (amountPaise / 100).toFixed(2),
      currency: "INR",
      debtor: {
        account_name: "ONDC NODAL SETTLEMENT ESCROW",
        account_number: "99001122334455",
        ifsc: "YESB0000001",
      },
      creditor: {
        account_name: "DIRECT BPP SELLER OPERATIVE",
        account_number: beneficiaryAccount,
        ifsc: beneficiaryIfsc,
      },
      remittance_info: "ONDC RSF 2.0 MULTI-PARTY SETTLEMENT NET DISBURSEMENT",
      iso20022_format: "pacs.008.001.08",
      idempotency_key: `idemp-wire-${Date.now()}`,
    };

    return NextResponse.json({
      receivedAtIST: baseTime.toISOString(),
      isPastCutoff,
      rtgsCutoffTime: "17:00:00 IST",
      targetSettlementWindow: `T+${tPlusDays}`,
      scheduledSettlementDate: finalSettlementDate.toISOString().split("T")[0],
      rolloverExplanations: rolloverLog,
      wireInstruction: wireInstructionPayload,
      upcomingBankHolidays: Object.entries(STATUTORY_HOLIDAYS_2026).slice(0, 6).map(([date, name]) => ({
        date,
        name,
        citation: "RBI Negotiable Instruments Act, 1881",
      })),
    });
  } catch (err: any) {
    return NextResponse.json(
      { error: "Settlement window calculation failed", details: err.message },
      { status: 500 }
    );
  }
}
