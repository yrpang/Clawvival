import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useDashboardQueries } from "./useDashboardQueries";

const useQueryMock = vi.fn();
const useInfiniteQueryMock = vi.fn();

vi.mock("@tanstack/react-query", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-query")>("@tanstack/react-query");
  return {
    ...actual,
    useQuery: (...args: unknown[]) => useQueryMock(...args),
    useInfiniteQuery: (...args: unknown[]) => useInfiniteQueryMock(...args),
  };
});

describe("useDashboardQueries", () => {
  beforeEach(() => {
    useQueryMock.mockReset();
    useInfiniteQueryMock.mockReset();

    useQueryMock.mockReturnValue({
      isFetching: false,
      dataUpdatedAt: 0,
      refetch: vi.fn(),
    });
    useInfiniteQueryMock.mockReturnValue({
      isFetching: false,
      dataUpdatedAt: 0,
      refetch: vi.fn(),
      hasNextPage: false,
      isFetchingNextPage: false,
      fetchNextPage: vi.fn(),
    });
  });

  it("does not set interval polling for replay infinite query", () => {
    renderHook(() => useDashboardQueries("agt_1"));

    expect(useInfiniteQueryMock).toHaveBeenCalledTimes(1);
    const options = useInfiniteQueryMock.mock.calls[0][0] as Record<string, unknown>;
    expect(options.refetchInterval).toBeUndefined();
  });
});
