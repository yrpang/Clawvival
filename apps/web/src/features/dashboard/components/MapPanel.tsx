import type { ObserveResponse, ObserveTile, Point } from "../../../types";
import {
  distanceFromOrigin,
  manhattan,
  tileVisualClasses,
  tileHighlightClass,
  tileMarkerSymbol,
  zoneByDistance,
} from "../model";

type PositionHighlight = {
  before?: Point | null;
  after?: Point | null;
};

type MapPanelProps = {
  observe: ObserveResponse | undefined;
  xRange: number[];
  yRange: number[];
  tileMap: Map<string, ObserveTile>;
  resourceMap: Map<string, { type: string; is_depleted: boolean }>;
  objectMap: Map<string, Array<{ id: string; type: string }>>;
  highlight: PositionHighlight;
  hasMovement: boolean;
  movementArrow: string;
  effectiveSelectedTileId: string | null;
  selectedTile: ObserveTile | undefined;
  selectedResource: { type: string; is_depleted: boolean } | undefined;
  selectedObjects: Array<{ id: string; type: string }>;
  tileDetailCorner: string;
  operableRadius: number;
  onSelectTile: (tileId: string) => void;
};

export function MapPanel({
  observe,
  xRange,
  yRange,
  tileMap,
  resourceMap,
  objectMap,
  highlight,
  hasMovement,
  movementArrow,
  effectiveSelectedTileId,
  selectedTile,
  selectedResource,
  selectedObjects,
  tileDetailCorner,
  operableRadius,
  onSelectTile,
}: MapPanelProps) {
  const cornerClass =
    tileDetailCorner === "bottom-right"
      ? "right-2 bottom-2"
      : tileDetailCorner === "bottom-left"
        ? "left-2 bottom-2"
        : tileDetailCorner === "top-right"
          ? "right-2 top-2"
          : "left-2 top-2";

  return (
    <section className="min-h-[75vh] rounded-[14px] border border-[var(--line)] bg-[var(--panel)] p-3 transition-[background,border-color,color] duration-200 max-[1080px]:min-h-0 [.theme-night_&]:border-[#2f3a5a] [.theme-night_&]:bg-[linear-gradient(180deg,rgba(21,28,44,0.96),rgba(26,30,52,0.92))] [.theme-night_&]:text-[#e6eefc]">
      <div className="mb-2 flex items-baseline justify-between">
        <h2>Map</h2>
        <span>threat {observe?.local_threat_level ?? "-"}</span>
      </div>
      <div className="mb-[10px] flex flex-wrap gap-x-3 gap-y-2 text-[0.82rem] text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-[3px] border border-[#bcae98] bg-[#ddebf8]" />safe (d{"<="}6)</span>
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-[3px] border border-[#bcae98] bg-[#d8edcb]" />forest (7-20)</span>
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-[3px] border border-[#bcae98] bg-[#ece3d6]" />quarry (21-35)</span>
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-[3px] border border-[#bcae98] bg-[#efd2c8]" />wild ({">"}35)</span>
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-full bg-[#f04f2f] shadow-[0_0_0_2px_#ffd7cf]" />agent</span>
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-full bg-[#3767c8] opacity-85" />visible</span>
        <span className="inline-flex items-center gap-1.5"><i className="inline-block h-[11px] w-[11px] rounded-full border-2 border-dashed border-[#c47f21] bg-transparent [.theme-night_&]:border-[#5ad1da]" />operable (auto: {observe?.time_of_day === "night" ? "night d<=1" : "day d<=2"})</span>
        <span className="inline-flex items-center gap-1.5">move: arrow {"->"} ●</span>
      </div>
      {!observe && <p className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">等待地图数据...</p>}
      {observe && (
        <div className="relative">
          <div className="grid gap-1 transition-[background,border-color] duration-200 [.theme-night_&]:rounded-[10px] [.theme-night_&]:border [.theme-night_&]:border-[#33415f] [.theme-night_&]:bg-[radial-gradient(circle_at_50%_0%,rgba(129,155,255,0.18),transparent_58%),linear-gradient(180deg,rgba(11,17,34,0.9),rgba(17,22,39,0.9))] [.theme-night_&]:p-2">
            <div className="grid grid-cols-[34px_repeat(11,minmax(0,1fr))] gap-1">
              <div className="grid place-items-center rounded-md border border-[#e7dac6] bg-[#f7f1e5] text-[0.78rem] text-[var(--muted)] [.theme-night_&]:border-[#334669] [.theme-night_&]:bg-[#1a2540] [.theme-night_&]:text-[#b4c2de]" />
              {xRange.map((x) => (
                <div
                  key={`x-${x}`}
                  className="grid place-items-center rounded-md border border-[#e7dac6] bg-[#f7f1e5] text-[0.78rem] text-[var(--muted)] [.theme-night_&]:border-[#334669] [.theme-night_&]:bg-[#1a2540] [.theme-night_&]:text-[#b4c2de]"
                >
                  {x}
                </div>
              ))}
            </div>
            {yRange.map((y) => (
              <div key={`row-${y}`} className="grid grid-cols-[34px_repeat(11,minmax(0,1fr))] gap-1">
                <div className="grid place-items-center rounded-md border border-[#e7dac6] bg-[#f7f1e5] text-[0.78rem] text-[var(--muted)] [.theme-night_&]:border-[#334669] [.theme-night_&]:bg-[#1a2540] [.theme-night_&]:text-[#b4c2de]">{y}</div>
                {xRange.map((x) => {
                  const key = `${x}:${y}`;
                  const tile = tileMap.get(key);
                  if (!tile) {
                    return (
                      <div
                        key={key}
                        className="min-h-[44px] rounded-[7px] border border-[#d9ccbe] bg-[#f6f6f6] [.theme-night_&]:border-[rgba(94,113,154,0.85)] [.theme-night_&]:bg-[#1a2540]"
                      />
                    );
                  }
                  const isAgent = tile.pos.x === observe.agent_state.position.x && tile.pos.y === observe.agent_state.position.y;
                  const isSelected = effectiveSelectedTileId === key;
                  const isVisible = tile.is_visible;
                  const dist = manhattan(tile.pos, observe.agent_state.position);
                  const isOperable = dist <= operableRadius;
                  const resource = resourceMap.get(key);
                  const objects = objectMap.get(key) ?? [];
                  const object = objects[0];
                  const objectTag = object ? `O:${object.type}${objects.length > 1 ? "..." : ""}` : null;
                  const tileHighlight = tileHighlightClass(tile.pos, highlight);
                  const marker = tileMarkerSymbol(tile.pos, {
                    agentPos: observe.agent_state.position,
                    highlight,
                    hasMovement,
                    movementArrow,
                  });
                  return (
                    <button
                      key={key}
                      className={tileVisualClasses({
                        tile,
                        highlightClass: tileHighlight,
                        isSelected,
                        isAgent,
                        isVisible,
                        isOperable,
                      })}
                      title={`${tile.terrain_type} (${tile.pos.x},${tile.pos.y})`}
                      aria-label={`tile ${tile.terrain_type} (${tile.pos.x},${tile.pos.y})${isAgent ? ", agent" : ""}${isSelected ? ", selected" : ""}`}
                      aria-pressed={isSelected}
                      onClick={() => onSelectTile(key)}
                    >
                      <div className="absolute left-1/2 top-1/2 z-[2] -translate-x-1/2 -translate-y-[56%] font-['IBM_Plex_Mono',monospace] leading-none">
                        {isAgent ? "A" : marker}
                      </div>
                      <div className="absolute bottom-0.5 left-0.5 right-0.5 z-[1] grid gap-px">
                        {resource && (
                          <span className="overflow-hidden text-ellipsis whitespace-nowrap text-left font-['IBM_Plex_Mono',monospace] text-[0.62rem] leading-none text-[#0d6758] [.theme-night_&]:text-[#7de0d0]">
                            R:{resource.type}{resource.is_depleted ? "*" : ""}
                          </span>
                        )}
                        {objectTag && (
                          <span className="overflow-hidden text-ellipsis whitespace-nowrap text-left font-['IBM_Plex_Mono',monospace] text-[0.62rem] leading-none text-[#513b12] [.theme-night_&]:text-[#8eb7ff]">
                            {objectTag}
                          </span>
                        )}
                      </div>
                    </button>
                  );
                })}
              </div>
            ))}
          </div>
          <section
            className={`${cornerClass} ${selectedTile ? "block" : "hidden"} absolute z-[7] max-h-[42%] w-[min(62%,560px)] overflow-auto rounded-[10px] border border-[rgba(206,180,146,0.75)] bg-[rgba(255,253,248,0.92)] p-2.5 backdrop-blur-[2px] [.theme-night_&]:border-[rgba(105,135,195,0.7)] [.theme-night_&]:bg-[rgba(24,34,58,0.86)] max-[1080px]:static max-[1080px]:mt-2.5 max-[1080px]:max-h-none max-[1080px]:w-full`}
          >
            <h3 className="mb-1.5 text-[0.95rem]">Tile Detail</h3>
            {selectedTile ? (
              <dl className="grid grid-cols-2 gap-x-3.5 gap-y-1.5">
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Coord</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">({selectedTile.pos.x}, {selectedTile.pos.y})</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Distance</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{distanceFromOrigin(selectedTile.pos)}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Zone</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{zoneByDistance(selectedTile.pos)}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Terrain</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{selectedTile.terrain_type}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Walkable</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{String(selectedTile.is_walkable)}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Visible</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{String(selectedTile.is_visible)}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Lit</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{String(selectedTile.is_lit)}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2"><dt>Resource</dt><dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{selectedResource ? `${selectedResource.type} (${selectedResource.is_depleted ? "depleted" : "ready"})` : "-"}</dd></div>
                <div className="grid grid-cols-[78px_minmax(0,1fr)] items-start gap-2">
                  <dt>Object</dt>
                  <dd className="text-[0.9rem] leading-[1.2] [word-break:normal] [overflow-wrap:anywhere]">{selectedObjects.length > 0 ? selectedObjects.map((obj) => obj.type).join(", ") : "-"}</dd>
                </div>
              </dl>
            ) : (
              <p className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">点击地图格子查看详情。</p>
            )}
          </section>
        </div>
      )}
    </section>
  );
}
