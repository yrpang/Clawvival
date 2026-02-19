import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useAgentUrlState } from "./useAgentUrlState";

describe("useAgentUrlState", () => {
  it("syncs agent id from browser history popstate", () => {
    window.history.replaceState(null, "", "/?agent_id=agt_a");
    const onApply = vi.fn();
    const { result } = renderHook(() => useAgentUrlState(onApply));

    expect(result.current.agentId).toBe("agt_a");

    act(() => {
      window.history.pushState(null, "", "/?agent_id=agt_b");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });

    expect(result.current.agentId).toBe("agt_b");
    expect(result.current.agentInput).toBe("agt_b");
  });
});
