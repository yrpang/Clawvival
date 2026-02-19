import { percent, vitalClass } from "../model";
import type { AgentState } from "../../../types";

type AgentStatePanelProps = {
  state: AgentState | undefined;
};

export function AgentStatePanel({ state }: AgentStatePanelProps) {
  return (
    <aside className="min-h-[75vh] overflow-auto rounded-[14px] border border-[var(--line)] bg-[var(--panel)] p-3 transition-[background,border-color,color] duration-200 max-[1080px]:min-h-0 [.theme-night_&]:border-[#2f3a5a] [.theme-night_&]:bg-[linear-gradient(180deg,rgba(21,28,44,0.96),rgba(26,30,52,0.92))] [.theme-night_&]:text-[#e6eefc]">
      <h2 className="mb-2">Agent State</h2>
      {!state && <p className="text-[var(--muted)] [.theme-night_&]:text-[#b4c2de]">请输入 agent_id 后加载。</p>}
      {state && (
        <>
          <div className="mb-3 grid gap-2">
            <div className="grid grid-cols-[56px_1fr_36px] items-center gap-2">
              <span>HP</span>
              <progress className={vitalClass(state.vitals.hp)} value={percent(state.vitals.hp)} max={100} />
              <strong>{state.vitals.hp}</strong>
            </div>
            <div className="grid grid-cols-[56px_1fr_36px] items-center gap-2">
              <span>Hunger</span>
              <progress className={vitalClass(state.vitals.hunger)} value={percent(state.vitals.hunger)} max={100} />
              <strong>{state.vitals.hunger}</strong>
            </div>
            <div className="grid grid-cols-[56px_1fr_36px] items-center gap-2">
              <span>Energy</span>
              <progress className={vitalClass(state.vitals.energy)} value={percent(state.vitals.energy)} max={100} />
              <strong>{state.vitals.energy}</strong>
            </div>
          </div>
          <dl className="grid gap-1.5">
            <div className="grid grid-cols-[90px_1fr] gap-1.5"><dt className="text-[var(--muted)]">Agent</dt><dd className="m-0 break-all">{state.agent_id}</dd></div>
            <div className="grid grid-cols-[90px_1fr] gap-1.5"><dt className="text-[var(--muted)]">Session</dt><dd className="m-0 break-all">{state.session_id ?? "-"}</dd></div>
            <div className="grid grid-cols-[90px_1fr] gap-1.5"><dt className="text-[var(--muted)]">Position</dt><dd className="m-0 break-all">({state.position.x}, {state.position.y})</dd></div>
            <div className="grid grid-cols-[90px_1fr] gap-1.5"><dt className="text-[var(--muted)]">Zone</dt><dd className="m-0 break-all">{state.current_zone ?? "-"}</dd></div>
            <div className="grid grid-cols-[90px_1fr] gap-1.5"><dt className="text-[var(--muted)]">Inventory</dt><dd className="m-0 break-all">{state.inventory_used}/{state.inventory_capacity}</dd></div>
            <div className="grid grid-cols-[90px_1fr] gap-1.5"><dt className="text-[var(--muted)]">Dead</dt><dd className="m-0 break-all">{String(state.dead)}</dd></div>
          </dl>

          <h3 className="mt-3">Ongoing</h3>
          <p className="mono m-0">
            {state.ongoing_action
              ? `${state.ongoing_action.type} -> ${new Date(state.ongoing_action.end_at).toLocaleTimeString()}`
              : "none"}
          </p>

          <h3 className="mt-3">Inventory</h3>
          <ul className="m-0 flex list-none flex-wrap gap-1.5 p-0">
            {Object.entries(state.inventory).map(([name, count]) => (
              <li key={name} className="rounded-full border border-[#ebdbc3] bg-[#f5ede1] px-2.5 py-1 text-[0.85rem] [.theme-night_&]:border-[#43527f] [.theme-night_&]:bg-[#2a3555] [.theme-night_&]:text-[#e8f1ff]">{name}: {count}</li>
            ))}
            {Object.keys(state.inventory).length === 0 && <li className="rounded-full border border-[#ebdbc3] bg-[#f5ede1] px-2.5 py-1 text-[0.85rem] [.theme-night_&]:border-[#43527f] [.theme-night_&]:bg-[#2a3555] [.theme-night_&]:text-[#e8f1ff]">empty</li>}
          </ul>
        </>
      )}
    </aside>
  );
}
