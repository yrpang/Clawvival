import type { InfiniteData, UseInfiniteQueryResult, UseQueryResult } from "@tanstack/react-query";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { fetchObserve, fetchReplay, fetchStatus } from "../../../lib/api";
import type { ObserveResponse, ReplayResponse, StatusResponse } from "../../../types";
import { DEFAULT_REFRESH_MS, REPLAY_FETCH_LIMIT } from "../model";

type UseDashboardQueriesResult = {
  statusQuery: UseQueryResult<StatusResponse, Error>;
  observeQuery: UseQueryResult<ObserveResponse, Error>;
  replayQuery: UseInfiniteQueryResult<InfiniteData<ReplayResponse, unknown>, Error>;
  isRefreshing: boolean;
  lastRefreshMs: number;
  refreshNow: () => void;
};

export function useDashboardQueries(agentId: string): UseDashboardQueriesResult {
  const enabled = agentId.trim().length > 0;

  const statusQuery = useQuery({
    queryKey: ["status", agentId],
    queryFn: () => fetchStatus(agentId),
    enabled,
    refetchInterval: DEFAULT_REFRESH_MS,
  });

  const observeQuery = useQuery({
    queryKey: ["observe", agentId],
    queryFn: () => fetchObserve(agentId),
    enabled,
    refetchInterval: DEFAULT_REFRESH_MS,
  });

  const replayQuery = useInfiniteQuery({
    queryKey: ["replay", agentId],
    queryFn: ({ pageParam }) =>
      fetchReplay(agentId, {
        limit: REPLAY_FETCH_LIMIT,
        occurredTo: typeof pageParam === "number" ? pageParam : undefined,
      }),
    enabled,
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
    staleTime: DEFAULT_REFRESH_MS,
  });

  const isRefreshing = statusQuery.isFetching || observeQuery.isFetching || replayQuery.isFetching;
  const lastRefreshMs = Math.max(
    statusQuery.dataUpdatedAt || 0,
    observeQuery.dataUpdatedAt || 0,
    replayQuery.dataUpdatedAt || 0,
  );

  function refreshNow() {
    if (!enabled) {
      return;
    }
    void Promise.all([
      statusQuery.refetch(),
      observeQuery.refetch(),
      replayQuery.refetch(),
    ]);
  }

  return {
    statusQuery,
    observeQuery,
    replayQuery,
    isRefreshing,
    lastRefreshMs,
    refreshNow,
  };
}
