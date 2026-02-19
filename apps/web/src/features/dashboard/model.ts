import type { ActionHistoryItem, ObserveTile, Point } from "../../types";

export const PAGE_SIZE = 20;
export const REPLAY_FETCH_LIMIT = 200;
export const DEFAULT_REFRESH_MS = 60_000;

export function prettyTime(ts: string): string {
  const date = new Date(ts);
  return Number.isNaN(date.getTime()) ? ts : date.toLocaleString();
}

export function percent(value: number): number {
  return Math.max(0, Math.min(100, value));
}

export function vitalClass(value: number): string {
  if (value <= 20) return "is-critical";
  if (value <= 50) return "is-warning";
  return "is-good";
}

export function tileKey(point: Point): string {
  return `${point.x}:${point.y}`;
}

export function distanceFromOrigin(point: Point): number {
  return Math.abs(point.x) + Math.abs(point.y);
}

export function zoneByDistance(point: Point): "safe" | "forest" | "quarry" | "wild" {
  const d = distanceFromOrigin(point);
  if (d <= 6) return "safe";
  if (d <= 20) return "forest";
  if (d <= 35) return "quarry";
  return "wild";
}

type TileVisualState = {
  tile: ObserveTile;
  isSelected: boolean;
  isAgent: boolean;
  isVisible: boolean;
  isOperable: boolean;
  highlightClass: "" | "highlight-before" | "highlight-after";
};

export function tileVisualClasses(state: TileVisualState): string {
  const zone = zoneByDistance(state.tile.pos);
  const classes = [
    "tile",
    `zone-${zone}`,
    state.tile.is_lit ? "lit" : "dim",
    "relative block min-h-[44px] overflow-hidden rounded-[7px] border border-[#d9ccbe] px-[2px] py-[3px] font-bold text-[#2f3440]",
    "[.theme-night_&]:border-[rgba(94,113,154,0.85)] [.theme-night_&]:text-[#f1f6ff]",
  ];

  if (zone === "safe") classes.push("bg-[#ddebf8] [.theme-night_&]:bg-[#2a3f67]");
  if (zone === "forest") classes.push("bg-[#d8edcb] [.theme-night_&]:bg-[#264a42]");
  if (zone === "quarry") classes.push("bg-[#ece3d6] [.theme-night_&]:bg-[#4a3f4c]");
  if (zone === "wild") classes.push("bg-[#efd2c8] [.theme-night_&]:bg-[#542f3f]");

  if (!state.tile.is_lit) {
    classes.push("opacity-[0.58] [.theme-night_&]:opacity-45");
  }
  if (state.highlightClass === "highlight-before") {
    classes.push(
      "highlight-before bg-[linear-gradient(180deg,rgba(255,173,111,0.55),rgba(255,146,74,0.38))] shadow-[inset_0_0_0_2px_rgba(185,104,42,0.72)]",
      "[.theme-night_&]:bg-[linear-gradient(180deg,rgba(255,184,126,0.5),rgba(240,144,79,0.34))] [.theme-night_&]:shadow-[inset_0_0_0_2px_rgba(226,154,88,0.75)]",
    );
  }
  if (state.highlightClass === "highlight-after") {
    classes.push(
      "highlight-after bg-[linear-gradient(180deg,rgba(145,240,175,0.5),rgba(96,214,135,0.36))] shadow-[inset_0_0_0_2px_rgba(31,143,79,0.72)]",
      "[.theme-night_&]:bg-[linear-gradient(180deg,rgba(122,236,176,0.44),rgba(74,201,150,0.3))] [.theme-night_&]:shadow-[inset_0_0_0_2px_rgba(99,219,147,0.78)]",
    );
  }
  if (state.isSelected) {
    classes.push(
      "selected outline outline-1 outline-[rgba(17,24,39,0.35)] outline-offset-0 bg-[linear-gradient(180deg,rgba(255,244,168,0.42),rgba(255,237,132,0.22))]",
      "[.theme-night_&]:outline-[rgba(217,230,255,0.35)] [.theme-night_&]:bg-[linear-gradient(180deg,rgba(131,158,255,0.35),rgba(97,126,235,0.2))]",
    );
  }
  if (state.isAgent) {
    classes.push(
      "agent-tile -translate-y-px shadow-[inset_0_0_0_3px_#f04f2f,0_0_0_2px_rgba(240,79,47,0.25)]",
      "[.theme-night_&]:shadow-[inset_0_0_0_3px_#ff855f,0_0_0_2px_rgba(255,120,98,0.25),0_0_14px_rgba(255,122,96,0.25)]",
    );
  }
  if (state.isVisible) {
    classes.push(
      "in-visible border-2 border-[rgba(55,103,200,0.9)]",
      "[.theme-night_&]:border-[rgba(123,160,255,0.98)]",
    );
  } else {
    classes.push("out-visible grayscale-[0.18]");
  }
  if (state.isOperable) {
    classes.push(
      "in-operable border-2 border-dashed border-[rgba(196,127,33,0.98)]",
      "[.theme-night_&]:border-[rgba(90,209,218,0.98)] [.theme-night_&]:bg-[linear-gradient(180deg,rgba(90,209,218,0.26),rgba(90,209,218,0.12))]",
    );
  }
  return classes.join(" ");
}

export function manhattan(a: Point, b: Point): number {
  return Math.abs(a.x - b.x) + Math.abs(a.y - b.y);
}

export function operableRadiusByTimeOfDay(timeOfDay: string): number {
  return timeOfDay === "night" ? 1 : 2;
}

export function directionArrow(from: Point, to: Point): string {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  if (dx === 0 && dy < 0) return "↑";
  if (dx === 0 && dy > 0) return "↓";
  if (dx < 0 && dy === 0) return "←";
  if (dx > 0 && dy === 0) return "→";
  if (dx > 0 && dy < 0) return "↗";
  if (dx < 0 && dy < 0) return "↖";
  if (dx > 0 && dy > 0) return "↘";
  if (dx < 0 && dy > 0) return "↙";
  return "•";
}

export function tileHighlightClass(
  tilePos: Point,
  highlight: { before?: Point | null; after?: Point | null },
): "" | "highlight-before" | "highlight-after" {
  const isBefore = highlight.before?.x === tilePos.x && highlight.before?.y === tilePos.y;
  const isAfter = highlight.after?.x === tilePos.x && highlight.after?.y === tilePos.y;
  if (isAfter) return "highlight-after";
  if (isBefore) return "highlight-before";
  return "";
}

export function tileMarkerSymbol(
  tilePos: Point,
  params: {
    agentPos: Point;
    highlight: { before?: Point | null; after?: Point | null };
    hasMovement: boolean;
    movementArrow: string;
  },
): string {
  const isAgent = tilePos.x === params.agentPos.x && tilePos.y === params.agentPos.y;
  if (isAgent) return "A";
  const isBefore = params.highlight.before?.x === tilePos.x && params.highlight.before?.y === tilePos.y;
  const isAfter = params.highlight.after?.x === tilePos.x && params.highlight.after?.y === tilePos.y;
  if (isBefore && params.hasMovement) return params.movementArrow;
  if (isAfter && params.hasMovement) return "●";
  if (isAfter) return "+";
  if (isBefore) return "-";
  return "";
}

export function getVitalsDelta(item: ActionHistoryItem): { hp: number; hunger: number; energy: number } {
  const result = item.result ?? {};
  const vitals = result.vitals_delta as Record<string, unknown> | undefined;
  return {
    hp: typeof vitals?.hp === "number" ? vitals.hp : 0,
    hunger: typeof vitals?.hunger === "number" ? vitals.hunger : 0,
    energy: typeof vitals?.energy === "number" ? vitals.energy : 0,
  };
}

export function signNum(value: number): string {
  return value > 0 ? `+${value}` : String(value);
}

function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value === "object" && value !== null) {
    return value as Record<string, unknown>;
  }
  return {};
}

export function inventoryDeltaSummary(item: ActionHistoryItem): string {
  const result = asRecord(item.result);
  const delta = asRecord(result.inventory_delta);
  const entries = Object.entries(delta).filter(([, value]) => typeof value === "number" && value !== 0);
  if (entries.length === 0) {
    return "no inventory change";
  }
  return entries
    .map(([key, value]) => `${key} ${signNum(value as number)}`)
    .join(", ");
}

export function formatRefreshTime(ms: number): string {
  if (!ms || ms <= 0) {
    return "-";
  }
  return new Date(ms).toLocaleTimeString();
}

export function utcOffsetLabel(): string {
  const minutesWest = new Date().getTimezoneOffset();
  const total = -minutesWest;
  const sign = total >= 0 ? "+" : "-";
  const abs = Math.abs(total);
  const hh = String(Math.floor(abs / 60)).padStart(2, "0");
  const mm = String(abs % 60).padStart(2, "0");
  return `UTC${sign}${hh}:${mm}`;
}

export function worldTimeDeltaLabel(before: number, after: number): string {
  const delta = after - before;
  return `${delta >= 0 ? "+" : ""}${delta}s`;
}

export function tileDetailCorner(selectedPos: Point | undefined, center: Point | undefined): string {
  if (!selectedPos || !center) {
    return "bottom-right";
  }
  const vertical = selectedPos.y <= center.y ? "bottom" : "top";
  const horizontal = selectedPos.x <= center.x ? "right" : "left";
  return `${vertical}-${horizontal}`;
}
