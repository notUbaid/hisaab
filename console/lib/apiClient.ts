import { ParticipantPerspective } from "./rbac";

export interface ApiError {
  message: string;
  code?: string;
  status: number;
  details?: any;
}

export interface RateLimitTelemetry {
  limit: number;
  remaining: number;
  resetAt: number;
  circuitBreakerOpen: boolean;
  activeParticipant: string;
}

export type RateLimitInfo = RateLimitTelemetry;

type TelemetryListener = (telemetry: RateLimitTelemetry) => void;

class ApiClient {
  private baseGoApiUrl: string;
  private currentRole: ParticipantPerspective = "COLLECTOR_RSP";
  private telemetry: RateLimitTelemetry = {
    limit: 60,
    remaining: 58,
    resetAt: Date.now() + 60000,
    circuitBreakerOpen: false,
    activeParticipant: "collector.ondc.org",
  };
  private listeners: Set<TelemetryListener> = new Set();

  constructor() {
    this.baseGoApiUrl =
      typeof window !== "undefined"
        ? "" // In browser, call Next.js API routes or proxy
        : process.env.HISAAB_API_URL || "http://127.0.0.1:8080";
  }

  public setRole(role: ParticipantPerspective) {
    this.currentRole = role;
    this.telemetry.activeParticipant =
      role === "BUYER_APP_BAP"
        ? "buyer-app.ondc.org"
        : role === "SELLER_APP_BPP"
        ? "seller-app.ondc.org"
        : "collector.ondc.org";
    this.notify();
  }

  public getRole(): ParticipantPerspective {
    return this.currentRole;
  }

  public getTelemetry(): RateLimitTelemetry {
    return { ...this.telemetry };
  }

  public subscribeTelemetry(listener: TelemetryListener): () => void {
    this.listeners.add(listener);
    listener(this.getTelemetry());
    return () => this.listeners.delete(listener);
  }

  private notify() {
    const current = this.getTelemetry();
    this.listeners.forEach((l) => l(current));
  }

  public updateRateLimits(headers: Headers) {
    const limitHeader = headers.get("X-RateLimit-Limit");
    const remainingHeader = headers.get("X-RateLimit-Remaining");
    const resetHeader = headers.get("X-RateLimit-Reset");

    if (limitHeader) this.telemetry.limit = parseInt(limitHeader, 10);
    if (remainingHeader) {
      this.telemetry.remaining = parseInt(remainingHeader, 10);
      this.telemetry.circuitBreakerOpen = this.telemetry.remaining <= 0;
    }
    if (resetHeader) {
      const resetSec = parseInt(resetHeader, 10);
      this.telemetry.resetAt = Date.now() + resetSec * 1000;
    }
    this.notify();
  }

  public async request<T = any>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const headers = new Headers(options.headers || {});
    headers.set("Content-Type", "application/json");
    headers.set("X-Hisaab-Role", this.currentRole);
    headers.set("X-Participant-Id", this.telemetry.activeParticipant);

    // If circuit breaker is tripped due to rate limit exhaustion (ADR-003)
    if (this.telemetry.circuitBreakerOpen && this.telemetry.resetAt > Date.now()) {
      const waitSeconds = Math.ceil((this.telemetry.resetAt - Date.now()) / 1000);
      throw {
        message: `Rate limit backpressure active: quota exhausted. Retry after ${waitSeconds}s`,
        code: "RATE_LIMIT_EXCEEDED",
        status: 429,
      } as ApiError;
    }

    let url = endpoint;
    if (!endpoint.startsWith("http") && !endpoint.startsWith("/")) {
      url = `/${endpoint}`;
    }

    try {
      const res = await fetch(url, {
        ...options,
        headers,
      });

      this.updateRateLimits(res.headers);

      if (!res.ok) {
        let errBody: any;
        try {
          errBody = await res.json();
        } catch {
          errBody = { message: res.statusText };
        }

        const apiErr: ApiError = {
          message: errBody.error || errBody.message || `Request failed with HTTP ${res.status}`,
          code: errBody.code || `HTTP_${res.status}`,
          status: res.status,
          details: errBody.details || errBody,
        };
        throw apiErr;
      }

      return (await res.json()) as T;
    } catch (err: any) {
      if (err.status) {
        throw err;
      }
      // Network or unexpected failure
      const netErr: ApiError = {
        message: err.message || "Network request failed. Go settlement engine unreachable.",
        code: "NETWORK_ERROR",
        status: 503,
      };
      throw netErr;
    }
  }

  public async get<T = any>(endpoint: string, headers?: Record<string, string>): Promise<T> {
    return this.request<T>(endpoint, { method: "GET", headers });
  }

  public async post<T = any>(endpoint: string, body?: any, headers?: Record<string, string>): Promise<T> {
    return this.request<T>(endpoint, {
      method: "POST",
      body: body ? JSON.stringify(body) : undefined,
      headers,
    });
  }

  public async put<T = any>(endpoint: string, body?: any, headers?: Record<string, string>): Promise<T> {
    return this.request<T>(endpoint, {
      method: "PUT",
      body: body ? JSON.stringify(body) : undefined,
      headers,
    });
  }

  public async delete<T = any>(endpoint: string, headers?: Record<string, string>): Promise<T> {
    return this.request<T>(endpoint, { method: "DELETE", headers });
  }

  // --- Domain Methods ---

  // Scenarios
  public async getScenarios(): Promise<{ source: string; count: number; scenarios: any[] }> {
    return this.get("/api/scenarios");
  }

  public async runScenario(
    scenarioId: string,
    targetMode: "simulator" | "live_api" = "simulator",
    failpoint: string = "FP_NONE"
  ): Promise<any> {
    return this.post("/api/scenarios/run", { scenarioId, targetMode, failpoint });
  }

  // Reconciliation & Exceptions
  public async getExceptions(): Promise<{ source: string; count: number; exceptions: any[] }> {
    return this.get("/api/recon");
  }

  public async approveReconException(params: {
    exceptionId: string;
    orderId?: string;
    npPair?: string;
    discrepancyPaise?: number;
    reason?: string;
  }): Promise<any> {
    return this.post("/api/recon", {
      action: "approve_tolerance",
      ...params,
    });
  }

  public async triggerRefund(params: {
    orderId: string;
    refundAmountPaise: number;
    npPair?: string;
    reason?: string;
  }): Promise<any> {
    return this.post("/api/recon", {
      action: "trigger_refund",
      ...params,
    });
  }

  // ONDC Registry
  public async getRegistryLookup(subscriberId: string, uniqueKeyId?: string): Promise<any> {
    const params = new URLSearchParams({ subscriber_id: subscriberId });
    if (uniqueKeyId) params.append("unique_key_id", uniqueKeyId);
    return this.get(`/api/registry?${params.toString()}`);
  }

  // Worker Telemetry
  public async getWorkerTelemetry(): Promise<any> {
    return this.get("/api/worker/telemetry");
  }

  // Settlements Window & Banking Calendar
  public async getSettlementWindow(payload: {
    submissionTime?: string;
    tPlusDays?: number;
    amountPaise?: number;
    beneficiaryIfsc?: string;
    beneficiaryAccount?: string;
  }): Promise<any> {
    return this.post("/api/settlements/window", payload);
  }

  // NP Toolkit & Tax Validator
  public async validateToolkit(payload: {
    orderAmountPaise?: number;
    commissionPaise?: number;
    bffPaise?: number;
    hasPan?: boolean;
    rawPayloadJson?: any;
  }): Promise<any> {
    return this.post("/api/toolkit", payload);
  }

  // Cryptographic Merkle Audit Verification
  public async verifyProof(payload: {
    scenarioId?: string;
    leafIndex?: number;
    tamperedLeafData?: string;
    journalEntries?: string[];
  }): Promise<any> {
    return this.post("/api/audit/verify", payload);
  }
}

export const apiClient = new ApiClient();
