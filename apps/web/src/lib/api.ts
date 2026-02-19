import type { ObserveResponse, ReplayResponse, StatusResponse } from "../types";

const API_BASE_RAW = import.meta.env.VITE_API_BASE_URL ?? "https://api.clawvival.app/";
const API_BASE = API_BASE_RAW.replace(/\/+$/, "");

async function requestJson<T>(path: string, init?: RequestInit): Promise<T> {
  const hasBody = init?.body !== undefined && init?.body !== null;
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      ...(hasBody ? { "Content-Type": "application/json" } : {}),
      ...(init?.headers ?? {}),
    },
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(`API ${response.status}: ${text}`);
  }

  return (await response.json()) as T;
}

export function fetchObserve(agentId: string): Promise<ObserveResponse> {
  return requestJson<ObserveResponse>("/api/agent/observe", {
    method: "POST",
    body: JSON.stringify({ agent_id: agentId }),
  });
}

export function fetchStatus(agentId: string): Promise<StatusResponse> {
  return requestJson<StatusResponse>("/api/agent/status", {
    method: "POST",
    body: JSON.stringify({ agent_id: agentId }),
  });
}

export async function fetchReplay(
  agentId: string,
  options?: { limit?: number; occurredTo?: number },
): Promise<ReplayResponse> {
  const limit = options?.limit ?? 200;
  const query = new URLSearchParams({ agent_id: agentId, limit: String(limit) });
  if (typeof options?.occurredTo === "number" && options.occurredTo > 0) {
    query.set("occurred_to", String(options.occurredTo));
  }
  return requestJson<ReplayResponse>(`/api/agent/replay?${query.toString()}`);
}
