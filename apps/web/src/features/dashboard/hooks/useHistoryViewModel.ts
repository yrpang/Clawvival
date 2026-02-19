import { useEffect, useMemo } from "react";
import { buildActionHistory, extractActionPositions, filterActionHistory } from "../../../lib/history";
import type { ActionHistoryItem, DomainEvent } from "../../../types";
import { directionArrow, PAGE_SIZE } from "../model";

type Params = {
  replayPages: Array<{ events: DomainEvent[] }> | undefined;
  actionFilter: string;
  fromTime: string;
  toTime: string;
  page: number;
  expandedId: string | null;
  hasNextReplayPage: boolean;
  isFetchingNextReplayPage: boolean;
  fetchNextReplayPage: () => Promise<unknown>;
};

type UseHistoryViewModelResult = {
  history: ActionHistoryItem[];
  pageCount: number;
  currentPage: number;
  pageItems: ActionHistoryItem[];
  expandedItem: ActionHistoryItem | undefined;
  highlight: { before: { x: number; y: number } | null; after: { x: number; y: number } | null };
  hasMovement: boolean;
  movementArrow: string;
};

export function useHistoryViewModel({
  replayPages,
  actionFilter,
  fromTime,
  toTime,
  page,
  expandedId,
  hasNextReplayPage,
  isFetchingNextReplayPage,
  fetchNextReplayPage,
}: Params): UseHistoryViewModelResult {
  const replayEvents = useMemo<DomainEvent[]>(() => {
    const seen = new Set<string>();
    const out: DomainEvent[] = [];
    const pages = replayPages ?? [];
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
  }, [replayPages]);

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

  return {
    history,
    pageCount,
    currentPage,
    pageItems,
    expandedItem,
    highlight,
    hasMovement,
    movementArrow,
  };
}
