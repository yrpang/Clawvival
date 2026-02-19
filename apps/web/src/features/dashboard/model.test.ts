import { describe, expect, it } from "vitest";
import { tileDetailCorner, tileHighlightClass, tileMarkerSymbol, tileVisualClasses, worldTimeDeltaLabel } from "./model";

describe("dashboard model", () => {
  it("places tile detail in opposite corner by quadrant", () => {
    const center = { x: 0, y: 0 };
    expect(tileDetailCorner({ x: -1, y: -1 }, center)).toBe("bottom-right");
    expect(tileDetailCorner({ x: 2, y: -1 }, center)).toBe("bottom-left");
    expect(tileDetailCorner({ x: -1, y: 2 }, center)).toBe("top-right");
    expect(tileDetailCorner({ x: 3, y: 4 }, center)).toBe("top-left");
  });

  it("formats world time delta with sign", () => {
    expect(worldTimeDeltaLabel(100, 160)).toBe("+60s");
    expect(worldTimeDeltaLabel(160, 100)).toBe("-60s");
    expect(worldTimeDeltaLabel(100, 100)).toBe("+0s");
  });

  it("computes tile marker and highlight classes", () => {
    const agentPos = { x: 0, y: 0 };
    const highlight = { before: { x: 1, y: 0 }, after: { x: 2, y: 0 } };
    expect(tileHighlightClass({ x: 1, y: 0 }, highlight)).toBe("highlight-before");
    expect(tileHighlightClass({ x: 2, y: 0 }, highlight)).toBe("highlight-after");
    expect(tileMarkerSymbol({ x: 0, y: 0 }, { agentPos, highlight, hasMovement: true, movementArrow: "→" })).toBe("A");
    expect(tileMarkerSymbol({ x: 1, y: 0 }, { agentPos, highlight, hasMovement: true, movementArrow: "→" })).toBe("→");
    expect(tileMarkerSymbol({ x: 2, y: 0 }, { agentPos, highlight, hasMovement: true, movementArrow: "→" })).toBe("●");
  });

  it("builds tile visual class list from state flags", () => {
    const classes = tileVisualClasses({
      tile: {
        pos: { x: 1, y: 1 },
        terrain_type: "grass",
        is_walkable: true,
        is_lit: true,
        is_visible: true,
      },
      highlightClass: "highlight-after",
      isSelected: true,
      isAgent: false,
      isVisible: true,
      isOperable: true,
    });
    expect(classes).toContain("tile");
    expect(classes).toContain("zone-safe");
    expect(classes).toContain("lit");
    expect(classes).toContain("highlight-after");
    expect(classes).toContain("selected");
    expect(classes).toContain("in-visible");
    expect(classes).toContain("in-operable");
  });
});
