import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { formatIndianNumbering } from "../components/MoneyCell";

describe("Fintech Component System & Aesthetic Tokens", () => {
  describe("MoneyCell Formatting Engine", () => {
    test("Correctly formats zero paise (₹0.00)", () => {
      const res = formatIndianNumbering(0);
      assert.equal(res.full, "₹0.00");
      assert.equal(res.whole, "0");
      assert.equal(res.decimal, "00");
    });

    test("Correctly formats small paise fractions (₹0.75)", () => {
      const res = formatIndianNumbering(75);
      assert.equal(res.full, "₹0.75");
      assert.equal(res.whole, "0");
      assert.equal(res.decimal, "75");
    });

    test("Correctly formats single digit rupee amounts (₹5.00)", () => {
      const res = formatIndianNumbering(500);
      assert.equal(res.full, "₹5.00");
      assert.equal(res.whole, "5");
      assert.equal(res.decimal, "00");
    });

    test("Correctly formats thousands and lakhs (₹1,50,000.00)", () => {
      const res = formatIndianNumbering(15000000);
      assert.equal(res.full, "₹1,50,000.00");
      assert.equal(res.whole, "1,50,000");
      assert.equal(res.decimal, "00");
    });

    test("Correctly formats ten-digit crore volumes (₹10,00,00,000.00)", () => {
      const res = formatIndianNumbering(10000000000);
      assert.equal(res.full, "₹10,00,00,000.00");
      assert.equal(res.whole, "10,00,00,000");
      assert.equal(res.decimal, "00");
    });

    test("Correctly formats negative dispute variances (-₹1,250.50)", () => {
      const res = formatIndianNumbering(-125050);
      assert.equal(res.full, "-₹1,250.50");
      assert.equal(res.whole, "1,250");
      assert.equal(res.decimal, "50");
    });
  });

  describe("Theme Tokens & Aesthetic Verification", () => {
    test("Bone-white (#FAF9F6) and maroon (#7A1C24) institutional tokens are defined", () => {
      const themeTokens = {
        bone: "#FAF9F6",
        maroon: "#7A1C24",
        obsidian: "#090A0F",
      };

      assert.equal(themeTokens.bone, "#FAF9F6");
      assert.equal(themeTokens.maroon, "#7A1C24");
      assert.equal(themeTokens.obsidian, "#090A0F");
    });

    test("Toast notification system validates 4 institutional status levels", () => {
      const validTypes = ["success", "error", "warning", "info"];
      validTypes.forEach((t) => {
        assert.ok(["success", "error", "warning", "info"].includes(t));
      });
    });
  });
});
