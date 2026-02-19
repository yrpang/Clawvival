import { useRef, useState } from "react";
import type { ActionHistoryItem } from "../../../types";
import { Badge } from "../../../shared/ui/Badge";
import { Button } from "../../../shared/ui/Button";
import { Input } from "../../../shared/ui/Input";
import { Card } from "../../../shared/ui/Card";
import { cn } from "../../../shared/lib/cn";
import {
  getVitalsDelta,
  inventoryDeltaSummary,
  prettyTime,
  signNum,
  worldTimeDeltaLabel,
} from "../model";

type HistoryPanelProps = {
  historyCount: number;
  hasMoreHistory: boolean;
  actionFilter: string;
  fromTime: string;
  toTime: string;
  onActionFilterChange: (value: string) => void;
  onFromTimeChange: (value: string) => void;
  onToTimeChange: (value: string) => void;
  isReplayError: boolean;
  replayErrorText: string;
  isFetchingNextPage: boolean;
  pageItems: ActionHistoryItem[];
  expandedId: string | null;
  onToggleExpand: (id: string) => void;
  currentPage: number;
  pageCount: number;
  onPrevPage: () => void;
  onNextPage: () => void;
  canPrevPage: boolean;
  canNextPage: boolean;
  expandedItem: ActionHistoryItem | undefined;
};

export function HistoryPanel({
  historyCount,
  hasMoreHistory,
  actionFilter,
  fromTime,
  toTime,
  onActionFilterChange,
  onFromTimeChange,
  onToTimeChange,
  isReplayError,
  replayErrorText,
  isFetchingNextPage,
  pageItems,
  expandedId,
  onToggleExpand,
  currentPage,
  pageCount,
  onPrevPage,
  onNextPage,
  canPrevPage,
  canNextPage,
  expandedItem,
}: HistoryPanelProps) {
  const listRef = useRef<HTMLUListElement | null>(null);
  const [detailAnchorClass, setDetailAnchorClass] = useState<"top-2" | "bottom-14">("top-2");

  function computeDetailAnchor(target: HTMLButtonElement) {
    const listEl = listRef.current;
    if (!listEl) {
      setDetailAnchorClass("top-2");
      return;
    }
    const listRect = listEl.getBoundingClientRect();
    const cardRect = target.getBoundingClientRect();
    const cardCenterY = cardRect.top + cardRect.height / 2;
    const visibleMidY = listRect.top + listRect.height / 2;
    setDetailAnchorClass(cardCenterY < visibleMidY ? "bottom-14" : "top-2");
  }

  return (
    <aside className="flex min-h-[75vh] min-w-0 flex-col rounded-[14px] border border-[var(--line)] bg-[var(--panel)] p-3 transition-[background,border-color,color] duration-200 max-[1080px]:min-h-0 [.theme-night_&]:border-[#2f3a5a] [.theme-night_&]:bg-[linear-gradient(180deg,rgba(21,28,44,0.96),rgba(26,30,52,0.92))] [.theme-night_&]:text-[#e6eefc]">
      <div className="mb-2 flex items-baseline justify-between">
        <h2>History</h2>
        <span>{historyCount} actions{hasMoreHistory ? "+" : ""}</span>
      </div>

      <div className="mb-2 grid gap-2">
        <Input
          className="rounded-[9px] px-[10px] py-2"
          value={actionFilter}
          onChange={(event) => onActionFilterChange(event.target.value)}
          placeholder="action type (gather/rest/...)"
          aria-label="history action type filter"
        />
        <label className="grid grid-cols-[42px_1fr] items-center gap-2 text-[0.9rem] text-[var(--muted)]">
          from
          <Input
            className="rounded-[9px] px-[10px] py-2"
            type="datetime-local"
            value={fromTime}
            onChange={(event) => onFromTimeChange(event.target.value)}
          />
        </label>
        <label className="grid grid-cols-[42px_1fr] items-center gap-2 text-[0.9rem] text-[var(--muted)]">
          to
          <Input
            className="rounded-[9px] px-[10px] py-2"
            type="datetime-local"
            value={toTime}
            onChange={(event) => onToTimeChange(event.target.value)}
          />
        </label>
      </div>
      {isReplayError && <p className="text-[var(--critical)]">{replayErrorText}</p>}
      {isFetchingNextPage && <p className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">加载更多历史中...</p>}
      {pageItems.length === 0 && <p className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">暂无 action_settled 记录。</p>}
      <div className="relative min-h-0">
        <ul ref={listRef} className="m-0 grid max-h-[48vh] min-h-0 list-none gap-1.5 overflow-auto p-0">
          {pageItems.map((item) => {
            const delta = getVitalsDelta(item);
            return (
              <li key={item.id}>
                <button
                  className={cn(
                    "w-full rounded-[10px] border border-[#ded4c5] bg-white p-[9px] text-left text-[var(--ink)]",
                    "grid grid-cols-[1fr_auto] gap-2.5",
                    "focus:outline-none focus-visible:ring-2 focus-visible:ring-[rgba(207,107,71,0.45)] focus-visible:ring-offset-0",
                    "[.theme-night_&]:focus-visible:ring-[rgba(130,183,255,0.5)]",
                    "[.theme-night_&]:border-[#4d5e8a] [.theme-night_&]:bg-[#1c253d] [.theme-night_&]:text-[#e6eefc]",
                    expandedId === item.id && "border-[#cf6b47] bg-[linear-gradient(180deg,#ffe4d8,#ffd6c4)] shadow-[inset_0_0_0_1px_rgba(255,255,255,0.35)]",
                    expandedId === item.id && "[.theme-night_&]:border-[#82b7ff] [.theme-night_&]:bg-[linear-gradient(180deg,#314e85,#273f70)] [.theme-night_&]:shadow-[inset_0_0_0_1px_rgba(158,199,255,0.4)]",
                  )}
                  aria-pressed={expandedId === item.id}
                  aria-label={`history ${item.action_type} at ${prettyTime(item.occurred_at)}`}
                  onClick={(event) => {
                    computeDetailAnchor(event.currentTarget);
                    onToggleExpand(item.id);
                  }}
                >
                  <div>
                    <strong>{item.action_type}</strong>
                    <small
                      className={cn(
                        "block text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]",
                        expandedId === item.id && "text-[#5a3022] [.theme-night_&]:text-[#deebff]",
                      )}
                      title={`${item.world_time_before_seconds}s -> ${item.world_time_after_seconds}s`}
                    >
                      {prettyTime(item.occurred_at)} ({worldTimeDeltaLabel(item.world_time_before_seconds, item.world_time_after_seconds)})
                    </small>
                  </div>
                  <div className="grid auto-rows-min content-start items-start justify-items-end gap-1">
                    <Badge tone={item.result_code.toLowerCase() === "ok" ? "ok" : item.result_code.toLowerCase() === "failed" ? "failed" : "neutral"}>
                      {item.result_code}
                    </Badge>
                    <small
                      className={cn(
                        "block text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]",
                        expandedId === item.id && "text-[#5a3022] [.theme-night_&]:text-[#deebff]",
                      )}
                    >
                      hp {signNum(delta.hp)} / hu {signNum(delta.hunger)} / en {signNum(delta.energy)}
                    </small>
                  </div>
                </button>
              </li>
            );
          })}
        </ul>

        <div className="mt-[10px] flex items-center justify-between">
          <Button disabled={!canPrevPage} onClick={onPrevPage}>Prev</Button>
          <span>{currentPage} / {hasMoreHistory ? `${pageCount}+` : pageCount}</span>
          <Button disabled={!canNextPage} onClick={onNextPage}>Next</Button>
        </div>

        <section
          className={cn(
            "absolute right-2 z-[8] mt-0 max-h-[min(62%,360px)] w-[min(88%,420px)] overflow-auto rounded-[10px] border border-[rgba(206,180,146,0.75)] bg-[rgba(255,253,248,0.95)] p-2.5 shadow-[0_6px_18px_rgba(23,30,46,0.12)] backdrop-blur-[2px]",
            "[.theme-night_&]:border-[rgba(105,135,195,0.72)] [.theme-night_&]:bg-[rgba(24,34,58,0.9)] [.theme-night_&]:shadow-[0_8px_20px_rgba(2,10,24,0.45)]",
            detailAnchorClass,
            expandedItem ? "block" : "hidden",
            "max-[1080px]:static max-[1080px]:mt-[10px] max-[1080px]:max-h-none max-[1080px]:w-full",
          )}
        >
          {expandedItem && (
            <>
              <h3>Action Detail</h3>
              <dl className="m-0 grid gap-1.5">
                <div className="grid grid-cols-[90px_1fr] gap-1.5">
                  <dt className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">Type</dt>
                  <dd className="m-0 break-all">{expandedItem.action_type}</dd>
                </div>
                <div className="grid grid-cols-[90px_1fr] gap-1.5">
                  <dt className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">Result</dt>
                  <dd className="m-0 break-all">{expandedItem.result_code}</dd>
                </div>
                <div className="grid grid-cols-[90px_1fr] gap-1.5">
                  <dt className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">At</dt>
                  <dd className="m-0 break-all">{prettyTime(expandedItem.occurred_at)}</dd>
                </div>
                <div className="grid grid-cols-[90px_1fr] gap-1.5">
                  <dt className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">World</dt>
                  <dd className="m-0 break-all">{expandedItem.world_time_before_seconds}s{" -> "}{expandedItem.world_time_after_seconds}s</dd>
                </div>
              </dl>
              <div className="mt-[10px] grid grid-cols-3 gap-2">
                <Card className="grid gap-1 rounded-[10px] border-[#ddcfbd] bg-[#fff8ec] p-2 [.theme-night_&]:border-[#43527f] [.theme-night_&]:bg-[#2a3555] [.theme-night_&]:text-[#e8f1ff]">
                  <span className="text-[0.82rem] text-[var(--muted)] [.theme-night_&]:text-[#b9c8e6]">HP</span>
                  <strong>{signNum(getVitalsDelta(expandedItem).hp)}</strong>
                </Card>
                <Card className="grid gap-1 rounded-[10px] border-[#ddcfbd] bg-[#fff8ec] p-2 [.theme-night_&]:border-[#43527f] [.theme-night_&]:bg-[#2a3555] [.theme-night_&]:text-[#e8f1ff]">
                  <span className="text-[0.82rem] text-[var(--muted)] [.theme-night_&]:text-[#b9c8e6]">Hunger</span>
                  <strong>{signNum(getVitalsDelta(expandedItem).hunger)}</strong>
                </Card>
                <Card className="grid gap-1 rounded-[10px] border-[#ddcfbd] bg-[#fff8ec] p-2 [.theme-night_&]:border-[#43527f] [.theme-night_&]:bg-[#2a3555] [.theme-night_&]:text-[#e8f1ff]">
                  <span className="text-[0.82rem] text-[var(--muted)] [.theme-night_&]:text-[#b9c8e6]">Energy</span>
                  <strong>{signNum(getVitalsDelta(expandedItem).energy)}</strong>
                </Card>
              </div>
              <p className="my-[10px] mb-[6px] text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]"><strong className="text-[var(--ink)] [.theme-night_&]:text-[#e6eefc]">Inventory:</strong> {inventoryDeltaSummary(expandedItem)}</p>
              <details>
                <summary className="mb-1 cursor-pointer text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">Raw Details</summary>
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
  );
}
