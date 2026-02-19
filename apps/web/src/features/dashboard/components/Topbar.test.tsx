import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Topbar } from "./Topbar";

describe("Topbar", () => {
  it("shows skills distribution link", () => {
    render(
      <Topbar
        agentInput=""
        onAgentInputChange={() => {}}
        onApplyAgent={() => {}}
        onRefresh={() => {}}
        isRefreshing={false}
        canRefresh={false}
        worldTimeSeconds="-"
        phase="-"
        nextPhaseInSeconds="-"
        lastRefreshText="-"
        timezone="+00:00"
      />,
    );

    const link = screen.getByRole("link", { name: /skills/i });
    expect(link.getAttribute("href")).toBe("/skills/index.html");
  });
});
