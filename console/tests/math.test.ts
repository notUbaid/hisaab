import { test, describe } from "node:test";
import assert from "node:assert/strict";

// Statutory RoundHalfEven implementation
function roundHalfEven(numerator: number, denominator: number): number {
  const quotient = Math.floor(numerator / denominator);
  const remainder = numerator % denominator;
  const twiceRemainder = remainder * 2;

  if (twiceRemainder > denominator) {
    return quotient + 1;
  } else if (twiceRemainder < denominator) {
    return quotient;
  } else {
    return quotient % 2 === 0 ? quotient : quotient + 1;
  }
}

// Indian Currency Numbering Formatter
function formatIndianNumbering(paise: number): string {
  const isNegative = paise < 0;
  const absPaise = Math.abs(paise);
  const rupees = Math.floor(absPaise / 100);
  const remainingPaise = absPaise % 100;

  const s = rupees.toString();
  let result = "";
  if (s.length <= 3) {
    result = s;
  } else {
    const last3 = s.slice(-3);
    const rest = s.slice(0, -3);
    const groups: string[] = [];
    for (let i = rest.length; i > 0; i -= 2) {
      const start = Math.max(0, i - 2);
      groups.unshift(rest.slice(start, i));
    }
    result = groups.join(",") + "," + last3;
  }

  const paiseStr = remainingPaise.toString().padStart(2, "0");
  return `${isNegative ? "-" : ""}₹${result}.${paiseStr}`;
}

describe("Statutory Banker's Rounding (RoundHalfEven)", () => {
  test("Rounds up when fractional part > 0.5", () => {
    // 60 / 100 = 0.6 -> 1
    assert.equal(roundHalfEven(60, 100), 1);
  });

  test("Rounds down when fractional part < 0.5", () => {
    // 40 / 100 = 0.4 -> 0
    assert.equal(roundHalfEven(40, 100), 0);
  });

  test("Rounds to nearest even integer when exactly half (0.5)", () => {
    // 25 / 10 = 2.5 -> nearest even is 2
    assert.equal(roundHalfEven(25, 10), 2);
    // 35 / 10 = 3.5 -> nearest even is 4
    assert.equal(roundHalfEven(35, 10), 4);
    // 45 / 10 = 4.5 -> nearest even is 4
    assert.equal(roundHalfEven(45, 10), 4);
  });
});

describe("Indian Currency Formatting (MoneyCell)", () => {
  test("Formats thousands (₹1,000.00)", () => {
    assert.equal(formatIndianNumbering(100000), "₹1,000.00");
  });

  test("Formats lakhs (₹1,24,500.50)", () => {
    assert.equal(formatIndianNumbering(12450050), "₹1,24,500.50");
  });

  test("Formats crores (₹1,00,00,000.00)", () => {
    assert.equal(formatIndianNumbering(1000000000), "₹1,00,00,000.00");
  });

  test("Handles negative paise (-₹500.00)", () => {
    assert.equal(formatIndianNumbering(-50000), "-₹500.00");
  });
});
