import { describe, expect, it } from "vitest";
import { buildActionHistory, extractActionPositions, filterActionHistory } from "./history";

describe("buildActionHistory", () => {
  it("extracts action_settled entries and sorts latest first", () => {
    const out = buildActionHistory([
      {
        type: "action_settled",
        occurred_at: "2026-02-19T10:00:00Z",
        payload: {
          world_time_before_seconds: 100,
          world_time_after_seconds: 400,
          decision: { intent: "rest", params: {} },
          state_before: { hp: 80 },
          state_after: { hp: 90 },
          result: { vitals_delta: { hp: 10 } },
        },
      },
      {
        type: "critical_hp",
        occurred_at: "2026-02-19T10:00:01Z",
        payload: {},
      },
      {
        type: "action_settled",
        occurred_at: "2026-02-19T11:00:00Z",
        payload: {
          world_time_before_seconds: 500,
          world_time_after_seconds: 800,
          decision: { intent: "gather", params: {} },
        },
      },
    ]);

    expect(out).toHaveLength(2);
    expect(out[0].action_type).toBe("gather");
    expect(out[0].result_code).toBe("OK");
    expect(out[0].world_time_before_seconds).toBe(500);
    expect(out[1].action_type).toBe("rest");
    expect(out[1].world_time_after_seconds).toBe(400);
  });

  it("filters by action type and time range", () => {
    const out = buildActionHistory([
      {
        type: "action_settled",
        occurred_at: "2026-02-19T10:00:00Z",
        payload: {
          world_time_before_seconds: 100,
          world_time_after_seconds: 400,
          decision: { intent: "rest", params: {} },
        },
      },
      {
        type: "action_settled",
        occurred_at: "2026-02-19T11:00:00Z",
        payload: {
          world_time_before_seconds: 500,
          world_time_after_seconds: 800,
          decision: { intent: "gather", params: {} },
        },
      },
      {
        type: "action_settled",
        occurred_at: "2026-02-19T12:00:00Z",
        payload: {
          world_time_before_seconds: 900,
          world_time_after_seconds: 1200,
          decision: { intent: "move", params: {} },
        },
      },
    ]);

    const filtered = filterActionHistory(out, {
      actionType: "gather",
      fromTime: "2026-02-19T10:30:00Z",
      toTime: "2026-02-19T11:30:00Z",
    });
    expect(filtered).toHaveLength(1);
    expect(filtered[0].action_type).toBe("gather");
  });

  it("extracts before and after positions for map highlighting", () => {
    const out = buildActionHistory([
      {
        type: "action_settled",
        occurred_at: "2026-02-19T10:00:00Z",
        payload: {
          world_time_before_seconds: 100,
          world_time_after_seconds: 400,
          decision: { intent: "move", params: {} },
          state_before: { x: 1, y: 2 },
          state_after: { x: 2, y: 2 },
        },
      },
    ]);

    const pos = extractActionPositions(out[0]);
    expect(pos.before).toEqual({ x: 1, y: 2 });
    expect(pos.after).toEqual({ x: 2, y: 2 });
  });
});
