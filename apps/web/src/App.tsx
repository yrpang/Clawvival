import "./index.css";
import { AgentStatePanel } from "./features/dashboard/components/AgentStatePanel";
import { HistoryPanel } from "./features/dashboard/components/HistoryPanel";
import { MapPanel } from "./features/dashboard/components/MapPanel";
import { Topbar } from "./features/dashboard/components/Topbar";
import {
  formatRefreshTime,
  operableRadiusByTimeOfDay,
  utcOffsetLabel,
} from "./features/dashboard/model";
import { useAgentUrlState } from "./features/dashboard/hooks/useAgentUrlState";
import { useDashboardQueries } from "./features/dashboard/hooks/useDashboardQueries";
import { useHistoryViewModel } from "./features/dashboard/hooks/useHistoryViewModel";
import { useMapViewModel } from "./features/dashboard/hooks/useMapViewModel";
import { useDashboardUiState } from "./features/dashboard/hooks/useDashboardUiState";

function App() {
  const {
    page,
    setPage,
    expandedId,
    setExpandedId,
    actionFilter,
    fromTime,
    toTime,
    selectedTileId,
    setSelectedTileId,
    resetForAgentChange,
    setActionFilterAndResetPage,
    setFromTimeAndResetPage,
    setToTimeAndResetPage,
    clearExpanded,
  } = useDashboardUiState();
  const { agentInput, setAgentInput, agentId, applyAgent } = useAgentUrlState(resetForAgentChange);
  const {
    statusQuery,
    observeQuery,
    replayQuery,
    isRefreshing,
    lastRefreshMs,
    refreshNow,
  } = useDashboardQueries(agentId);
  const observe = observeQuery.data;
  const state = observe?.agent_state ?? statusQuery.data?.agent_state;
  const timeOfDay = observe?.time_of_day ?? statusQuery.data?.time_of_day ?? "day";
  const operableRadius = operableRadiusByTimeOfDay(observe?.time_of_day ?? statusQuery.data?.time_of_day ?? "day");
  const {
    history,
    pageCount,
    currentPage,
    pageItems,
    expandedItem,
    highlight,
    hasMovement,
    movementArrow,
  } = useHistoryViewModel({
    replayPages: replayQuery.data?.pages,
    actionFilter,
    fromTime,
    toTime,
    page,
    expandedId,
    hasNextReplayPage: Boolean(replayQuery.hasNextPage),
    isFetchingNextReplayPage: replayQuery.isFetchingNextPage,
    fetchNextReplayPage: async () => replayQuery.fetchNextPage(),
  });
  const {
    tileMap,
    resourceMap,
    objectMap,
    xRange,
    yRange,
    effectiveSelectedTileId,
    selectedTile,
    selectedResource,
    selectedObjects,
    selectedCorner,
  } = useMapViewModel(observe, selectedTileId);
  const tzLabel = utcOffsetLabel();

  return (
    <main className={`min-h-screen p-[18px] transition-[background,color] duration-200 ${timeOfDay === "night" ? "theme-night" : "theme-day"}`}>
      <Topbar
        agentInput={agentInput}
        onAgentInputChange={setAgentInput}
        onApplyAgent={applyAgent}
        onRefresh={refreshNow}
        isRefreshing={isRefreshing}
        canRefresh={agentId.trim().length > 0}
        worldTimeSeconds={observe?.world_time_seconds ?? statusQuery.data?.world_time_seconds ?? "-"}
        phase={observe?.time_of_day ?? statusQuery.data?.time_of_day ?? "-"}
        nextPhaseInSeconds={observe?.next_phase_in_seconds ?? statusQuery.data?.next_phase_in_seconds ?? "-"}
        lastRefreshText={formatRefreshTime(lastRefreshMs)}
        timezone={tzLabel}
      />
      <section className="grid grid-cols-[280px_minmax(380px,1fr)_430px] gap-[14px] max-[1080px]:grid-cols-1">
        <AgentStatePanel state={state} />
        <MapPanel
          observe={observe}
          xRange={xRange}
          yRange={yRange}
          tileMap={tileMap}
          resourceMap={resourceMap}
          objectMap={objectMap}
          highlight={highlight}
          hasMovement={hasMovement}
          movementArrow={movementArrow}
          effectiveSelectedTileId={effectiveSelectedTileId}
          selectedTile={selectedTile}
          selectedResource={selectedResource}
          selectedObjects={selectedObjects}
          tileDetailCorner={selectedCorner}
          operableRadius={operableRadius}
          onSelectTile={(key) => setSelectedTileId((prev) => (prev === key ? null : key))}
        />
        <HistoryPanel
          historyCount={history.length}
          hasMoreHistory={Boolean(replayQuery.hasNextPage)}
          actionFilter={actionFilter}
          fromTime={fromTime}
          toTime={toTime}
          onActionFilterChange={(value) => {
            setActionFilterAndResetPage(value);
          }}
          onFromTimeChange={(value) => {
            setFromTimeAndResetPage(value);
          }}
          onToTimeChange={(value) => {
            setToTimeAndResetPage(value);
          }}
          isReplayError={replayQuery.isError}
          replayErrorText={String(replayQuery.error)}
          isFetchingNextPage={replayQuery.isFetchingNextPage}
          pageItems={pageItems}
          expandedId={expandedId}
          onToggleExpand={(id) => setExpandedId((prev) => (prev === id ? null : id))}
          currentPage={currentPage}
          pageCount={pageCount}
          onPrevPage={() => {
            setPage((p) => p - 1);
            clearExpanded();
          }}
          onNextPage={() => {
            setPage((p) => p + 1);
            clearExpanded();
          }}
          canPrevPage={currentPage > 1}
          canNextPage={Boolean(currentPage < pageCount || replayQuery.hasNextPage)}
          expandedItem={expandedItem}
        />
      </section>
    </main>
  );
}

export default App;
