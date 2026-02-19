export type Point = { x: number; y: number };

export type AgentState = {
  agent_id: string;
  session_id?: string;
  vitals: { hp: number; hunger: number; energy: number };
  position: Point;
  current_zone?: string;
  inventory: Record<string, number>;
  inventory_capacity: number;
  inventory_used: number;
  action_cooldowns?: Record<string, number>;
  status_effects?: string[];
  dead: boolean;
  death_cause: string;
  ongoing_action?: {
    type: string;
    minutes: number;
    end_at: string;
  } | null;
  updated_at: string;
};

export type ObserveTile = {
  pos: Point;
  terrain_type: string;
  is_walkable: boolean;
  is_lit: boolean;
  is_visible: boolean;
};

export type ObserveResponse = {
  agent_state: AgentState;
  world_time_seconds: number;
  time_of_day: string;
  next_phase_in_seconds: number;
  local_threat_level: number;
  view: {
    width: number;
    height: number;
    center: Point;
    radius: number;
  };
  tiles: ObserveTile[];
  resources: Array<{ id: string; type: string; pos: Point; is_depleted: boolean }>;
  objects: Array<{ id: string; type: string; pos: Point }>;
};

export type StatusResponse = {
  agent_state: AgentState;
  world_time_seconds: number;
  time_of_day: string;
  next_phase_in_seconds: number;
};

export type DomainEvent = {
  type: string;
  occurred_at: string;
  payload?: Record<string, unknown>;
};

export type ReplayResponse = {
  events: DomainEvent[];
  latest_state: AgentState;
};

export type ActionHistoryItem = {
  id: string;
  occurred_at: string;
  action_type: string;
  result_code: string;
  world_time_before_seconds: number;
  world_time_after_seconds: number;
  state_before?: Record<string, unknown>;
  state_after?: Record<string, unknown>;
  result?: Record<string, unknown>;
  payload: Record<string, unknown>;
};
