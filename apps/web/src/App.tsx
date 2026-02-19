import { useEffect, useMemo, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import "./index.css";
import { fetchObserve, fetchReplay, fetchStatus } from "./lib/api";
import { buildActionHistory, extractActionPositions, filterActionHistory } from "./lib/history";
import type { ActionHistoryItem, DomainEvent, ObserveTile, Point } from "./types";

const PAGE_SIZE = 20;
const REPLAY_FETCH_LIMIT = 200;
const DEFAULT_REFRESH_MS = 60_000;

function prettyTime(ts: string): string {
  const date = new Date(ts);
  return Number.isNaN(date.getTime()) ? ts : date.toLocaleString();
}

function percent(value: number): number {
  return Math.max(0, Math.min(100, value));
}

function vitalClass(value: number): string {
  if (value <= 20) return "is-critical";
  if (value <= 50) return "is-warning";
  return "is-good";
}

function tileClass(tile: ObserveTile): string {
  const base = `tile zone-${zoneByDistance(tile.pos)}`;
  return tile.is_lit ? `${base} lit` : `${base} dim`;
}

function tileKey(point: Point): string {
  return `${point.x}:${point.y}`;
}

function distanceFromOrigin(point: Point): number {
  return Math.abs(point.x) + Math.abs(point.y);
}

function zoneByDistance(point: Point): "safe" | "forest" | "quarry" | "wild" {
  const d = distanceFromOrigin(point);
  if (d <= 6) return "safe";
  if (d <= 20) return "forest";
  if (d <= 35) return "quarry";
  return "wild";
}

function manhattan(a: Point, b: Point): number {
  return Math.abs(a.x - b.x) + Math.abs(a.y - b.y);
}

function operableRadiusByTimeOfDay(timeOfDay: string): number {
  return timeOfDay === "night" ? 1 : 2;
}

function directionArrow(from: Point, to: Point): string {
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

function getVitalsDelta(item: ActionHistoryItem): { hp: number; hunger: number; energy: number } {
  const result = item.result ?? {};
  const vitals = result.vitals_delta as Record<string, unknown> | undefined;
  return {
    hp: typeof vitals?.hp === "number" ? vitals.hp : 0,
    hunger: typeof vitals?.hunger === "number" ? vitals.hunger : 0,
    energy: typeof vitals?.energy === "number" ? vitals.energy : 0,
  };
}

function signNum(value: number): string {
  return value > 0 ? `+${value}` : String(value);
}

function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value === "object" && value !== null) {
    return value as Record<string, unknown>;
  }
  return {};
}

function inventoryDeltaSummary(item: ActionHistoryItem): string {
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

function formatRefreshTime(ms: number): string {
  if (!ms || ms <= 0) {
    return "-";
  }
  return new Date(ms).toLocaleTimeString();
}

function utcOffsetLabel(): string {
  const minutesWest = new Date().getTimezoneOffset();
  const total = -minutesWest;
  const sign = total >= 0 ? "+" : "-";
  const abs = Math.abs(total);
  const hh = String(Math.floor(abs / 60)).padStart(2, "0");
  const mm = String(abs % 60).padStart(2, "0");
  return `UTC${sign}${hh}:${mm}`;
}

function worldTimeDeltaLabel(before: number, after: number): string {
  const delta = after - before;
  return `${delta >= 0 ? "+" : ""}${delta}s`;
}

function App() {
  const urlAtInit = new URL(window.location.href);
  const [agentInput, setAgentInput] = useState(() => {
    return urlAtInit.searchParams.get("agent_id") ?? "";
  });
  const [agentId, setAgentId] = useState(agentInput);
  const [page, setPage] = useState(1);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [actionFilter, setActionFilter] = useState("");
  const [fromTime, setFromTime] = useState("");
  const [toTime, setToTime] = useState("");
  const [selectedTileId, setSelectedTileId] = useState<string | null>(null);

  const statusQuery = useQuery({
    queryKey: ["status", agentId],
    queryFn: () => fetchStatus(agentId),
    enabled: agentId.trim().length > 0,
    refetchInterval: DEFAULT_REFRESH_MS,
  });

  const observeQuery = useQuery({
    queryKey: ["observe", agentId],
    queryFn: () => fetchObserve(agentId),
    enabled: agentId.trim().length > 0,
    refetchInterval: DEFAULT_REFRESH_MS,
  });

  const replayQuery = useInfiniteQuery({
    queryKey: ["replay", agentId],
    queryFn: ({ pageParam }) =>
      fetchReplay(agentId, {
        limit: REPLAY_FETCH_LIMIT,
        occurredTo: typeof pageParam === "number" ? pageParam : undefined,
      }),
    enabled: agentId.trim().length > 0,
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (lastPage) => {
      const events = lastPage.events;
      if (events.length < REPLAY_FETCH_LIMIT) {
        return undefined;
      }
      const oldest = events[events.length - 1];
      if (!oldest?.occurred_at) {
        return undefined;
      }
      const ts = Math.floor(new Date(oldest.occurred_at).getTime() / 1000);
      if (!Number.isFinite(ts) || ts <= 1) {
        return undefined;
      }
      return ts - 1;
    },
    refetchInterval: DEFAULT_REFRESH_MS,
  });

  const replayEvents = useMemo<DomainEvent[]>(() => {
    const seen = new Set<string>();
    const out: DomainEvent[] = [];
    const pages = replayQuery.data?.pages ?? [];
    for (const pageData of pages) {
      for (const event of pageData.events) {
        const key = `${event.type}|${event.occurred_at}|${JSON.stringify(event.payload ?? {})}`;
        if (seen.has(key)) {
          continue;
        }
        seen.add(key);
        out.push(event);
      }
    }
    return out;
  }, [replayQuery.data?.pages]);

  const fullHistory = useMemo(
    () => buildActionHistory(replayEvents),
    [replayEvents],
  );
  const history = useMemo(
    () => filterActionHistory(fullHistory, { actionType: actionFilter, fromTime, toTime }),
    [actionFilter, fromTime, toTime, fullHistory],
  );

  const pageCount = Math.max(1, Math.ceil(history.length / PAGE_SIZE));
  const currentPage = Math.min(page, pageCount);
  const pageItems = history.slice((currentPage - 1) * PAGE_SIZE, currentPage * PAGE_SIZE);
  const expandedItem: ActionHistoryItem | undefined = pageItems.find((item) => item.id === expandedId);
  const highlight = extractActionPositions(expandedItem);
  const hasMovement =
    !!highlight.before &&
    !!highlight.after &&
    (highlight.before.x !== highlight.after.x || highlight.before.y !== highlight.after.y);
  const movementArrow = hasMovement && highlight.before && highlight.after
    ? directionArrow(highlight.before, highlight.after)
    : "";
  const fetchNextReplayPage = replayQuery.fetchNextPage;
  const hasNextReplayPage = replayQuery.hasNextPage;
  const isFetchingNextReplayPage = replayQuery.isFetchingNextPage;

  useEffect(() => {
    const needCount = currentPage * PAGE_SIZE;
    if (history.length >= needCount) {
      return;
    }
    if (!hasNextReplayPage || isFetchingNextReplayPage) {
      return;
    }
    void fetchNextReplayPage();
  }, [currentPage, fetchNextReplayPage, hasNextReplayPage, history.length, isFetchingNextReplayPage]);

  const observe = observeQuery.data;
  const state = observe?.agent_state ?? statusQuery.data?.agent_state;
  const timeOfDay = observe?.time_of_day ?? statusQuery.data?.time_of_day ?? "day";
  const operableRadius = operableRadiusByTimeOfDay(observe?.time_of_day ?? statusQuery.data?.time_of_day ?? "day");
  const tileMap = useMemo(() => {
    const map = new Map<string, ObserveTile>();
    for (const tile of observe?.tiles ?? []) {
      map.set(tileKey(tile.pos), tile);
    }
    return map;
  }, [observe?.tiles]);
  const resourceMap = useMemo(() => {
    const map = new Map<string, { type: string; is_depleted: boolean }>();
    for (const resource of observe?.resources ?? []) {
      map.set(tileKey(resource.pos), { type: resource.type, is_depleted: resource.is_depleted });
    }
    return map;
  }, [observe?.resources]);
  const objectMap = useMemo(() => {
    const map = new Map<string, Array<{ id: string; type: string }>>();
    for (const obj of observe?.objects ?? []) {
      const key = tileKey(obj.pos);
      const prev = map.get(key) ?? [];
      prev.push({ id: obj.id, type: obj.type });
      map.set(key, prev);
    }
    return map;
  }, [observe?.objects]);
  const xRange = useMemo(() => {
    if (!observe) return [] as number[];
    const start = observe.view.center.x - observe.view.radius;
    const end = observe.view.center.x + observe.view.radius;
    return Array.from({ length: end - start + 1 }, (_, i) => start + i);
  }, [observe]);
  const yRange = useMemo(() => {
    if (!observe) return [] as number[];
    const start = observe.view.center.y - observe.view.radius;
    const end = observe.view.center.y + observe.view.radius;
    return Array.from({ length: end - start + 1 }, (_, i) => start + i);
  }, [observe]);
  const effectiveSelectedTileId =
    selectedTileId && tileMap.has(selectedTileId)
      ? selectedTileId
      : null;
  const selectedTile = effectiveSelectedTileId ? tileMap.get(effectiveSelectedTileId) : undefined;
  const selectedResource = selectedTile ? resourceMap.get(tileKey(selectedTile.pos)) : undefined;
  const selectedObjects = selectedTile ? (objectMap.get(tileKey(selectedTile.pos)) ?? []) : [];
  const tileDetailCorner =
    selectedTile && observe
      ? `${selectedTile.pos.y <= observe.view.center.y ? "bottom" : "top"}-${selectedTile.pos.x <= observe.view.center.x ? "right" : "left"}`
      : "bottom-right";
  const lastRefreshMs = Math.max(
    statusQuery.dataUpdatedAt || 0,
    observeQuery.dataUpdatedAt || 0,
    replayQuery.dataUpdatedAt || 0,
  );
  const tzLabel = utcOffsetLabel();
  const isRefreshing = statusQuery.isFetching || observeQuery.isFetching || replayQuery.isFetching;

  function refreshNow() {
    if (!agentId.trim()) {
      return;
    }
    void Promise.all([
      statusQuery.refetch(),
      observeQuery.refetch(),
      replayQuery.refetch(),
    ]);
  }

  function applyAgent() {
    const next = agentInput.trim();
    if (!next) return;
    const url = new URL(window.location.href);
    url.searchParams.set("agent_id", next);
    window.history.replaceState(null, "", url.toString());
    setAgentId(next);
    setPage(1);
    setExpandedId(null);
    setSelectedTileId(null);
    setActionFilter("");
    setFromTime("");
    setToTime("");
  }

  useEffect(() => {
    const url = new URL(window.location.href);
    if (agentId.trim()) {
      url.searchParams.set("agent_id", agentId.trim());
    }
    window.history.replaceState(null, "", url.toString());
  }, [agentId]);

  return (
    <main className={`app-shell ${timeOfDay === "night" ? "theme-night" : "theme-day"}`}>
      <header className="topbar">
        <div className="brand">
          <h1>Clawvival Agent Console</h1>
          <p>Public Read Dashboard</p>
        </div>
        <div className="agent-picker">
          <input
            value={agentInput}
            onChange={(event) => setAgentInput(event.target.value)}
            placeholder="agent_id"
            aria-label="agent id"
          />
          <button onClick={applyAgent}>Load</button>
          <button onClick={refreshNow} disabled={isRefreshing || !agentId.trim()}>
            {isRefreshing ? "Refreshing..." : "Refresh"}
          </button>
        </div>
        <div className="world-meta">
          <div>world: {observe?.world_time_seconds ?? statusQuery.data?.world_time_seconds ?? "-"}s</div>
          <div>phase: {observe?.time_of_day ?? statusQuery.data?.time_of_day ?? "-"}</div>
          <div>next: {observe?.next_phase_in_seconds ?? statusQuery.data?.next_phase_in_seconds ?? "-"}s</div>
          <div>last refresh: {formatRefreshTime(lastRefreshMs)}</div>
          <div>timezone: {tzLabel}</div>
        </div>
      </header>

      <section className="layout">
        <aside className="panel left">
          <h2>Agent State</h2>
          {!state && <p className="muted">请输入 agent_id 后加载。</p>}
          {state && (
            <>
              <div className="vitals">
                <div className="vital-row">
                  <span>HP</span>
                  <progress className={vitalClass(state.vitals.hp)} value={percent(state.vitals.hp)} max={100} />
                  <strong>{state.vitals.hp}</strong>
                </div>
                <div className="vital-row">
                  <span>Hunger</span>
                  <progress className={vitalClass(state.vitals.hunger)} value={percent(state.vitals.hunger)} max={100} />
                  <strong>{state.vitals.hunger}</strong>
                </div>
                <div className="vital-row">
                  <span>Energy</span>
                  <progress className={vitalClass(state.vitals.energy)} value={percent(state.vitals.energy)} max={100} />
                  <strong>{state.vitals.energy}</strong>
                </div>
              </div>
              <dl className="kv-list">
                <div><dt>Agent</dt><dd>{state.agent_id}</dd></div>
                <div><dt>Session</dt><dd>{state.session_id ?? "-"}</dd></div>
                <div><dt>Position</dt><dd>({state.position.x}, {state.position.y})</dd></div>
                <div><dt>Zone</dt><dd>{state.current_zone ?? "-"}</dd></div>
                <div><dt>Inventory</dt><dd>{state.inventory_used}/{state.inventory_capacity}</dd></div>
                <div><dt>Dead</dt><dd>{String(state.dead)}</dd></div>
              </dl>

              <h3>Ongoing</h3>
              <p className="mono">
                {state.ongoing_action
                  ? `${state.ongoing_action.type} -> ${new Date(state.ongoing_action.end_at).toLocaleTimeString()}`
                  : "none"}
              </p>

              <h3>Inventory</h3>
              <ul className="tag-list">
                {Object.entries(state.inventory).map(([name, count]) => (
                  <li key={name}>{name}: {count}</li>
                ))}
                {Object.keys(state.inventory).length === 0 && <li>empty</li>}
              </ul>
            </>
          )}
        </aside>

        <section className="panel center">
          <div className="panel-title-row">
            <h2>Map</h2>
            <span>threat {observe?.local_threat_level ?? "-"}</span>
          </div>
          <div className="zone-legend">
            <span><i className="legend-swatch zone-safe" />safe (d{"<="}6)</span>
            <span><i className="legend-swatch zone-forest" />forest (7-20)</span>
            <span><i className="legend-swatch zone-quarry" />quarry (21-35)</span>
            <span><i className="legend-swatch zone-wild" />wild ({">"}35)</span>
            <span><i className="legend-mark agent" />agent</span>
            <span><i className="legend-mark visible" />visible</span>
            <span><i className="legend-mark operable" />operable (auto: {observe?.time_of_day === "night" ? "night d<=1" : "day d<=2"})</span>
            <span>move: arrow {"->"} ●</span>
          </div>
          {!observe && <p className="muted">等待地图数据...</p>}
          {observe && (
            <>
              <div className="map-stage">
                <div className="map-board">
                  <div className="map-row axis">
                    <div className="coord corner" />
                    {xRange.map((x) => (
                      <div key={`x-${x}`} className="coord x">{x}</div>
                    ))}
                  </div>
                  {yRange.map((y) => (
                    <div key={`row-${y}`} className="map-row">
                      <div className="coord y">{y}</div>
                      {xRange.map((x) => {
                        const key = `${x}:${y}`;
                        const tile = tileMap.get(key);
                        if (!tile) {
                          return <div key={key} className="tile tile-empty" />;
                        }
                        const isAgent = tile.pos.x === observe.agent_state.position.x && tile.pos.y === observe.agent_state.position.y;
                        const isBefore = highlight.before?.x === tile.pos.x && highlight.before?.y === tile.pos.y;
                        const isAfter = highlight.after?.x === tile.pos.x && highlight.after?.y === tile.pos.y;
                        const isSelected = effectiveSelectedTileId === key;
                        const isVisible = tile.is_visible;
                        const dist = manhattan(tile.pos, observe.agent_state.position);
                        const isOperable = dist <= operableRadius;
                        const resource = resourceMap.get(key);
                        const objects = objectMap.get(key) ?? [];
                        const object = objects[0];
                        const objectTag = object ? `O:${object.type}${objects.length > 1 ? "..." : ""}` : null;
                        const tileHighlight = isAfter ? "highlight-after" : isBefore ? "highlight-before" : "";
                        return (
                          <button
                            key={key}
                            className={`${tileClass(tile)} ${tileHighlight} ${isSelected ? "selected" : ""} ${isAgent ? "agent-tile" : ""} ${isVisible ? "in-visible" : "out-visible"} ${isOperable ? "in-operable" : ""}`.trim()}
                            title={`${tile.terrain_type} (${tile.pos.x},${tile.pos.y})`}
                            onClick={() => setSelectedTileId((prev) => (prev === key ? null : key))}
                          >
                            <div className="tile-main">
                              {isAgent ? "A" : isBefore && hasMovement ? movementArrow : isAfter && hasMovement ? "●" : isAfter ? "+" : isBefore ? "-" : ""}
                            </div>
                            <div className="tile-tags">
                              {resource && <span className="tag-resource">R:{resource.type}{resource.is_depleted ? "*" : ""}</span>}
                              {objectTag && <span className="tag-object">{objectTag}</span>}
                            </div>
                          </button>
                        );
                      })}
                    </div>
                  ))}
                </div>
                <section className={`tile-detail tile-detail-overlay corner-${tileDetailCorner} ${selectedTile ? "is-open" : ""}`}>
                  <h3>Tile Detail</h3>
                  {selectedTile ? (
                    <dl className="kv-list tile-kv-list">
                      <div><dt>Coord</dt><dd>({selectedTile.pos.x}, {selectedTile.pos.y})</dd></div>
                      <div><dt>Distance</dt><dd>{distanceFromOrigin(selectedTile.pos)}</dd></div>
                      <div><dt>Zone</dt><dd>{zoneByDistance(selectedTile.pos)}</dd></div>
                      <div><dt>Terrain</dt><dd>{selectedTile.terrain_type}</dd></div>
                      <div><dt>Walkable</dt><dd>{String(selectedTile.is_walkable)}</dd></div>
                      <div><dt>Visible</dt><dd>{String(selectedTile.is_visible)}</dd></div>
                      <div><dt>Lit</dt><dd>{String(selectedTile.is_lit)}</dd></div>
                      <div><dt>Resource</dt><dd>{selectedResource ? `${selectedResource.type} (${selectedResource.is_depleted ? "depleted" : "ready"})` : "-"}</dd></div>
                      <div>
                        <dt>Object</dt>
                        <dd>{selectedObjects.length > 0 ? selectedObjects.map((obj) => obj.type).join(", ") : "-"}</dd>
                      </div>
                    </dl>
                  ) : (
                    <p className="muted">点击地图格子查看详情。</p>
                  )}
                </section>
              </div>
            </>
          )}
        </section>

        <aside className="panel right">
          <div className="panel-title-row">
            <h2>History</h2>
            <span>{history.length} actions{replayQuery.hasNextPage ? "+" : ""}</span>
          </div>
          <div className="history-filters">
            <input
              value={actionFilter}
              onChange={(event) => {
                setActionFilter(event.target.value);
                setPage(1);
                setExpandedId(null);
              }}
              placeholder="action type (gather/rest/...)"
              aria-label="history action type filter"
            />
            <label>
              from
              <input
                type="datetime-local"
                value={fromTime}
                onChange={(event) => {
                  setFromTime(event.target.value);
                  setPage(1);
                  setExpandedId(null);
                }}
              />
            </label>
            <label>
              to
              <input
                type="datetime-local"
                value={toTime}
                onChange={(event) => {
                  setToTime(event.target.value);
                  setPage(1);
                  setExpandedId(null);
                }}
              />
            </label>
          </div>
          {replayQuery.isError && <p className="error">{String(replayQuery.error)}</p>}
          {replayQuery.isFetchingNextPage && <p className="muted">加载更多历史中...</p>}
          {pageItems.length === 0 && <p className="muted">暂无 action_settled 记录。</p>}
          <div className={`history-stage ${expandedItem ? "has-open-detail" : ""}`}>
            <ul className="history-list">
              {pageItems.map((item) => {
                const delta = getVitalsDelta(item);
                return (
                  <li key={item.id}>
                    <button
                      className={`history-row ${expandedId === item.id ? "active" : ""}`}
                      onClick={() => setExpandedId((prev) => (prev === item.id ? null : item.id))}
                    >
                      <div>
                        <strong>{item.action_type}</strong>
                        <small title={`${item.world_time_before_seconds}s -> ${item.world_time_after_seconds}s`}>
                          {prettyTime(item.occurred_at)} ({worldTimeDeltaLabel(item.world_time_before_seconds, item.world_time_after_seconds)})
                        </small>
                      </div>
                      <div className="history-meta">
                        <small className={`result-code result-${item.result_code.toLowerCase()}`}>{item.result_code}</small>
                        <small>
                          hp {signNum(delta.hp)} / hu {signNum(delta.hunger)} / en {signNum(delta.energy)}
                        </small>
                      </div>
                    </button>
                  </li>
                );
              })}
            </ul>

            <div className="pager">
              <button disabled={currentPage <= 1} onClick={() => { setPage((p) => p - 1); setExpandedId(null); }}>Prev</button>
              <span>{currentPage} / {replayQuery.hasNextPage ? `${pageCount}+` : pageCount}</span>
              <button
                disabled={currentPage >= pageCount && !replayQuery.hasNextPage}
                onClick={() => { setPage((p) => p + 1); setExpandedId(null); }}
              >
                Next
              </button>
            </div>

            <section className={`history-detail history-detail-overlay ${expandedItem ? "is-open" : ""}`}>
              {expandedItem && (
                <>
                  <h3>Action Detail</h3>
                  <dl className="kv-list">
                    <div><dt>Type</dt><dd>{expandedItem.action_type}</dd></div>
                    <div><dt>Result</dt><dd>{expandedItem.result_code}</dd></div>
                    <div><dt>At</dt><dd>{prettyTime(expandedItem.occurred_at)}</dd></div>
                    <div><dt>World</dt><dd>{expandedItem.world_time_before_seconds}s{" -> "}{expandedItem.world_time_after_seconds}s</dd></div>
                  </dl>
                  <div className="detail-visuals">
                    <div className="metric-card">
                      <span>HP</span>
                      <strong>{signNum(getVitalsDelta(expandedItem).hp)}</strong>
                    </div>
                    <div className="metric-card">
                      <span>Hunger</span>
                      <strong>{signNum(getVitalsDelta(expandedItem).hunger)}</strong>
                    </div>
                    <div className="metric-card">
                      <span>Energy</span>
                      <strong>{signNum(getVitalsDelta(expandedItem).energy)}</strong>
                    </div>
                  </div>
                  <p className="detail-inline"><strong>Inventory:</strong> {inventoryDeltaSummary(expandedItem)}</p>
                  <details className="raw-details">
                    <summary>Raw Details</summary>
                    <h4>Result</h4>
                    <pre>{JSON.stringify(expandedItem.result, null, 2)}</pre>
                    <h4>Before</h4>
                    <pre>{JSON.stringify(expandedItem.state_before, null, 2)}</pre>
                    <h4>After</h4>
                    <pre>{JSON.stringify(expandedItem.state_after, null, 2)}</pre>
                  </details>
                </>
              )}
            </section>
          </div>
        </aside>
      </section>
    </main>
  );
}

export default App;
