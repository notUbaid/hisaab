import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { apiClient } from "../lib/apiClient";
import { PERSPECTIVES } from "../lib/rbac";

describe("API Client & Telemetry Engine", () => {
  test("Switches perspective role and updates participant identity", () => {
    apiClient.setRole("BUYER_APP_BAP");
    assert.equal(apiClient.getRole(), "BUYER_APP_BAP");
    assert.equal(apiClient.getTelemetry().activeParticipant, "buyer-app.ondc.org");

    apiClient.setRole("SELLER_APP_BPP");
    assert.equal(apiClient.getRole(), "SELLER_APP_BPP");
    assert.equal(apiClient.getTelemetry().activeParticipant, "seller-app.ondc.org");

    apiClient.setRole("COLLECTOR_RSP");
    assert.equal(apiClient.getRole(), "COLLECTOR_RSP");
    assert.equal(apiClient.getTelemetry().activeParticipant, "collector.ondc.org");
  });

  test("Updates rate-limiting telemetry from HTTP response headers", () => {
    const headers = new Headers();
    headers.set("X-RateLimit-Limit", "120");
    headers.set("X-RateLimit-Remaining", "42");
    headers.set("X-RateLimit-Reset", "30");

    apiClient.updateRateLimits(headers);
    const telemetry = apiClient.getTelemetry();

    assert.equal(telemetry.limit, 120);
    assert.equal(telemetry.remaining, 42);
    assert.equal(telemetry.circuitBreakerOpen, false);
    assert.ok(telemetry.resetAt > Date.now());
  });

  test("Trips circuit breaker when remaining rate limit drops to 0", () => {
    const headers = new Headers();
    headers.set("X-RateLimit-Limit", "120");
    headers.set("X-RateLimit-Remaining", "0");
    headers.set("X-RateLimit-Reset", "60");

    apiClient.updateRateLimits(headers);
    const telemetry = apiClient.getTelemetry();

    assert.equal(telemetry.remaining, 0);
    assert.equal(telemetry.circuitBreakerOpen, true);
  });

  test("Subscribes to telemetry notifications on change", () => {
    let capturedTelemetry: any = null;
    const unsubscribe = apiClient.subscribeTelemetry((t) => {
      capturedTelemetry = t;
    });

    const headers = new Headers();
    headers.set("X-RateLimit-Limit", "60");
    headers.set("X-RateLimit-Remaining", "55");
    apiClient.updateRateLimits(headers);

    assert.ok(capturedTelemetry);
    assert.equal(capturedTelemetry.remaining, 55);

    unsubscribe();
  });

  test("RBAC perspective map contains all 3 participant roles with required capabilities", () => {
    assert.ok(PERSPECTIVES.COLLECTOR_RSP);
    assert.ok(PERSPECTIVES.BUYER_APP_BAP);
    assert.ok(PERSPECTIVES.SELLER_APP_BPP);

    assert.equal(PERSPECTIVES.COLLECTOR_RSP.id, "COLLECTOR_RSP");
    assert.equal(PERSPECTIVES.BUYER_APP_BAP.id, "BUYER_APP_BAP");
    assert.equal(PERSPECTIVES.SELLER_APP_BPP.id, "SELLER_APP_BPP");
  });
});
