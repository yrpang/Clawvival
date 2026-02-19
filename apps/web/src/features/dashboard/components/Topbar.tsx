import { Button } from "../../../shared/ui/Button";
import { Input } from "../../../shared/ui/Input";

type TopbarProps = {
  agentInput: string;
  onAgentInputChange: (value: string) => void;
  onApplyAgent: () => void;
  onRefresh: () => void;
  isRefreshing: boolean;
  canRefresh: boolean;
  worldTimeSeconds: number | string;
  phase: string;
  nextPhaseInSeconds: number | string;
  lastRefreshText: string;
  timezone: string;
};

export function Topbar({
  agentInput,
  onAgentInputChange,
  onApplyAgent,
  onRefresh,
  isRefreshing,
  canRefresh,
  worldTimeSeconds,
  phase,
  nextPhaseInSeconds,
  lastRefreshText,
  timezone,
}: TopbarProps) {
  return (
    <header
      className="
        mb-[14px] grid grid-cols-1 gap-3 rounded-[14px] border border-[#d8d2c7] p-3
        bg-[linear-gradient(120deg,#fffaf1,#f6fbff)]
        lg:grid-cols-[minmax(240px,1fr)_minmax(280px,1.2fr)_minmax(240px,1fr)]
        [.theme-night_&]:border-[#2f3b5b]
        [.theme-night_&]:bg-[linear-gradient(120deg,rgba(21,32,55,0.94),rgba(29,32,62,0.92))]
      "
    >
      <div>
        <h1 className="m-0 text-[1.3rem] font-semibold">Clawvival Agent Console</h1>
        <p className="m-0 mt-1 text-[#626f83] [.theme-night_&]:text-[#b4c2de]">Public Read Dashboard</p>
        <a
          className="mt-1 inline-flex text-[0.9rem] font-semibold text-[#92521d] underline decoration-[#c27633] underline-offset-2 [.theme-night_&]:text-[#ffd28d] [.theme-night_&]:decoration-[#ffd28d]"
          href="/skills/index.html"
          target="_blank"
          rel="noreferrer"
        >
          Skills
        </a>
      </div>

      <div className="mx-auto grid w-full max-w-[760px] min-w-0 grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2">
        <Input
          className="border-[#d8d2c7]"
          value={agentInput}
          onChange={(event) => onAgentInputChange(event.target.value)}
          placeholder="agent_id"
          aria-label="agent id"
        />
        <Button onClick={onApplyAgent}>Load</Button>
        <Button onClick={onRefresh} disabled={isRefreshing || !canRefresh}>
          {isRefreshing ? "Refreshing..." : "Refresh"}
        </Button>
      </div>

      <div
        className="
          grid min-w-0 justify-self-stretch text-left text-[0.95rem] font-extrabold tracking-[0.01em] text-[#4f5f78]
          grid-cols-1 gap-y-1 lg:justify-self-end lg:grid-cols-2 lg:gap-x-4
          [.theme-night_&]:text-[#d4e2ff]
        "
      >
        <div className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">world: {worldTimeSeconds}s</div>
        <div className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">phase: {phase}</div>
        <div className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">next: {nextPhaseInSeconds}s</div>
        <div className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">last refresh: {lastRefreshText}</div>
        <div className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap">timezone: {timezone}</div>
      </div>
    </header>
  );
}
