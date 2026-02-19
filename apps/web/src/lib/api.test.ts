import { afterEach, describe, expect, it, vi } from "vitest";
import { fetchObserve, fetchReplay } from "./api";

describe("api request headers", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("sends content-type for POST observe", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({ agent_state: { position: { x: 0, y: 0 } }, view: { center: { x: 0, y: 0 }, radius: 1 }, tiles: [], resources: [], objects: [] }),
    } as Response);

    await fetchObserve("agt_1");

    const [, init] = fetchSpy.mock.calls[0];
    expect((init?.headers as Record<string, string>)["Content-Type"]).toBe("application/json");
  });

  it("does not send content-type for GET replay", async () => {
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue({
      ok: true,
      json: async () => ({ events: [], latest_state: {} }),
    } as Response);

    await fetchReplay("agt_1", { limit: 20 });

    const [, init] = fetchSpy.mock.calls[0];
    const headers = (init?.headers ?? {}) as Record<string, string>;
    expect(headers["Content-Type"]).toBeUndefined();
  });
});
