import type { ActionHistoryItem, DomainEvent, Point } from "../types";

function toNumber(value: unknown): number {
  if (typeof value === "number") {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : 0;
  }
  return 0;
}

function asRecord(value: unknown): Record<string, unknown> {
  if (typeof value === "object" && value !== null) {
    return value as Record<string, unknown>;
  }
  return {};
}

export function buildActionHistory(events: DomainEvent[]): ActionHistoryItem[] {
  const settled = events.filter((event) => event.type === "action_settled");

  return settled
    .map((event, index) => {
      const payload = asRecord(event.payload);
      const decision = asRecord(payload.decision);
      const params = asRecord(decision.params);
      const intent = typeof decision.intent === "string" ? decision.intent : "unknown";
      const actionType =
        intent === "sleep" && typeof params.bed_id === "string"
          ? `sleep (${params.bed_id})`
          : intent;
      const resultCode = typeof payload.result_code === "string" ? payload.result_code : "OK";

      return {
        id: `${event.occurred_at}-${index}`,
        occurred_at: event.occurred_at,
        action_type: actionType,
        result_code: resultCode,
        world_time_before_seconds: toNumber(payload.world_time_before_seconds),
        world_time_after_seconds: toNumber(payload.world_time_after_seconds),
        state_before: asRecord(payload.state_before),
        state_after: asRecord(payload.state_after),
        result: asRecord(payload.result),
        payload,
      };
    })
    .sort((a, b) => (a.occurred_at < b.occurred_at ? 1 : -1));
}

type HistoryFilter = {
  actionType: string;
  fromTime: string;
  toTime: string;
};

function asDateMs(value: string): number {
  if (!value) {
    return 0;
  }
  const ms = new Date(value).getTime();
  return Number.isFinite(ms) ? ms : 0;
}

export function filterActionHistory(items: ActionHistoryItem[], filter: HistoryFilter): ActionHistoryItem[] {
  const action = filter.actionType.trim().toLowerCase();
  const fromMs = asDateMs(filter.fromTime);
  const toMs = asDateMs(filter.toTime);

  return items.filter((item) => {
    if (action && !item.action_type.toLowerCase().startsWith(action)) {
      return false;
    }
    const ts = asDateMs(item.occurred_at);
    if (fromMs > 0 && ts < fromMs) {
      return false;
    }
    if (toMs > 0 && ts > toMs) {
      return false;
    }
    return true;
  });
}

function readPosition(value: Record<string, unknown> | undefined): Point | null {
  if (!value) {
    return null;
  }
  if (typeof value.x === "number" && typeof value.y === "number") {
    return { x: value.x, y: value.y };
  }
  const pos = asRecord(value.pos);
  if (typeof pos.x === "number" && typeof pos.y === "number") {
    return { x: pos.x, y: pos.y };
  }
  return null;
}

export function extractActionPositions(item?: ActionHistoryItem): { before: Point | null; after: Point | null } {
  if (!item) {
    return { before: null, after: null };
  }
  return {
    before: readPosition(item.state_before),
    after: readPosition(item.state_after),
  };
}
