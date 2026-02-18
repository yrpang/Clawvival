# Clawvival MVP 详细产品设计 v1.0（合并版）

> 本文档是对 v0.1 ~ v0.9 的**合并定稿**：
> - 以最终修正为准（后版本覆盖前版本）。
> - 目标：用最小复杂度打通 **可玩闭环**，并保证 **Agent 通过 API 清晰理解状态、可计算决策、可复盘**。
>
> 关键拍板（来自 @yrpang）：
> - MVP **不需要可胜战斗**（不提供 `combat`）。
> - 地图：**有限窗口** + **稀疏资源刷新**。
> - 火把/围墙：**可选**。

---

## 0) 范围与非目标

### 0.1 MVP 必须打通的可玩闭环

主循环（每次心跳/每次 agent 回合）：
- `观察(Observe) -> 评估 -> 决策 -> 执行(Action) -> 结算 -> 复盘(Events)`

玩法闭环（必须可玩）：
1) **生存三要素**：`HP/Hunger/Energy` 持续消耗、可恢复
2) **食物链**：
   - 临时补给：采集 `berry` -> `eat`
   - 稳态补给：`seed` -> `farm_plot` -> `wheat` -> `eat`
3) **休整链**：
   - `rest`/`sleep` -> 恢复 `energy`（并对 `hp` 有小幅恢复出口）
4) **资源与建造链**：
   - 采集 `wood/stone` -> 合成/建造 -> `bed/box/farm_plot`
5) **背包管理链**：
   - `container_deposit/withdraw`（箱子存取）

72h 定居 Gate（验收口径）：
- 在同一 `session_id` 内满足：**bed + box + farm_plot**（且至少一次 `farm_plant` 成功）。

### 0.2 MVP 明确不做 / 弱化

- 不做多 Agent 交互（协作/交易/对抗）。
- 不做科技树/自动化/复杂制造链。
- 不做“可胜战斗”，不提供 `combat`。
- 威胁系统仅做**可观测风险提示 + 撤离(retreat)**，不引入不可控暴毙（尤其不引入夜晚伤害倍率）。

---

## 1) 世界、空间、窗口与可见性（强契约，减少歧义）

### 1.1 绝对坐标系（写死）

- 世界坐标为整数网格：`(x, y)`，均为**绝对坐标**。
- 方向定义（写死）：
  - `N`（North）：`y + 1`
  - `S`（South）：`y - 1`
  - `E`（East）：`x + 1`
  - `W`（West）：`x - 1`

### 1.2 observe 窗口大小（写死）

- MVP 固定窗口：**11x11**（半径 5）。
- 以 Agent 当前坐标 `(x0,y0)` 为中心：
  - `x ∈ [x0-5, x0+5]`
  - `y ∈ [y0-5, y0+5]`

`observe` 必须返回：
- `view: { width: 11, height: 11, center:{x0,y0}, radius: 5 }`

### 1.3 窗口 vs 视野（语义区分）

- `view.radius=5`：**返回范围**（tiles 的空间边界）
- `vision_radius_day/night` 与 `torch_light_radius`：**信息可见性**（窗口内哪些 tile 的 `is_visible=true`）

`tiles[]` 返回窗口内所有 tile 的基础字段：
- `pos{x,y}`（绝对坐标）
- `terrain_type`
- `is_walkable`
- `is_lit`
- `is_visible`

实体返回规则（MVP 写死，最简单可解释）：
- `resources/objects/threats` **只会出现在** `is_visible=true` 的 tile 上。
- 对 `is_visible=false` 的 tile：仍返回 tile 基础信息，但不返回其上的实体。

---

## 2) 时间与结算（dt 连续结算 + 动作耗时/推进回传）

### 2.1 双层时间与 dt 约束（工程硬约束）

- Standard Tick：30 分钟（参数表达标尺）。
- `dt`：由服务端根据该 Agent “上次成功结算时间 -> 当前时间”计算，**客户端不得提交 dt**。
- 连续时间结算：
  - `实际变化量 = 配置值(每30mins) * (dt / 30)`

### 2.2 昼夜循环（世界时钟）

- 白天 10 分钟 / 夜晚 5 分钟（世界时间）。
- 单位一致性写死：
  - `world_time_seconds` 单调递增
  - `1 world minute = 60 world seconds`
  - `day_length_minutes=10` 与 `world_time_seconds` 换算固定

`observe/status` 暴露：
- `world_time_seconds`
- `time_of_day: day|night`
- `next_phase_in_seconds`
- `hp_drain_feedback`（是否正在掉血、预计每 30 分钟掉血、原因分解）

### 2.3 动作耗时与时间推进（Agent 决策闭环必需）

为支持 Agent 预估成本 + 执行后确认推进：

1) `observe/status` 暴露 `action_costs`（预估成本）
- `action_costs: { intent_type: { base_minutes, delta_hunger, delta_energy, requirements[] } }`

2) `action` 响应必须回传时间推进（确认）
- `settled_dt_minutes`
- `world_time_before_seconds`
- `world_time_after_seconds`（或 `advanced_seconds`）

---

## 3) 核心状态模型（HP/Hunger/Energy + 容量体系写死）

### 3.1 AgentState 最小字段

`agent_state`（observe/status 必含）：
- `hp: 0..100`
- `hunger: 0..100`
- `energy: 0..100`
- `position: {x,y}`
- `inventory: {item_type: count}`
- `inventory_capacity: int`
- `inventory_used: int`（与 capacity 同一计数规则：item 总数）
- `home?: {x,y,radius}`
- `session_id: string`

容量体系（写死）：
- MVP **只做** `inventory_capacity + inventory_used`，不提供 encumbrance。

### 3.2 关键阈值与派生标签（status_effects）

阈值必须显式暴露（避免 Agent 反推配置）：
- `critical_hp_threshold`（默认 15）
- `low_energy_threshold`（默认 20）

`status_effects: string[]`（派生解释层，减少 Agent 阈值分支代码）：
- MVP 必须实现：
  - `STARVING`（hunger==0）
  - `EXHAUSTED`（energy <= low_energy_threshold）
  - `CRITICAL`（hp <= critical_hp_threshold）
  - `IN_DARK`（time_of_day==night 且当前位置 is_lit==false）

---

## 4) 规则参数显式暴露（避免 Agent 反推服务器配置）

建议统一放到：
- `observe.world.rules`
- `status.world.rules`（结构一致；可选只返回同结构的子集，但字段名/层级一致）

`world.rules` 最小字段（MVP 必含）：
- `standard_tick_minutes: 30`
- `drains_per_30m`：
  - `hunger_drain`
  - `energy_drain`
  - `hp_drain_model`（`dynamic_capped`）
  - `hp_drain_from_hunger_coeff`（默认 `0.08`）
  - `hp_drain_from_energy_coeff`（默认 `0.05`）
  - `hp_drain_cap`（默认 `12`）
- `thresholds`：
  - `critical_hp`
  - `low_energy`
- `visibility`：
  - `vision_radius_day`
  - `vision_radius_night`
  - `torch_light_radius`
- `farming`：
  - `farm_grow_minutes`
  - `wheat_yield_range: [min,max]`（或固定值也行，但必须显式）
  - `seed_return_chance`
- `seed`（二选一实现，但参数必须可解释）：
  - 方案 A：`seed_drop_chance` + `seed_pity_max_fails` + `seed_pity_remaining?`
  - 方案 B：`seed_cache_enabled=true`（存在一次性 seed 点）

推荐默认范围（便于可玩与验收）：
- `hunger_drain_per_30m`: 4~6（建议 5）
- `energy_drain_per_30m`: 3~5（建议 4）
- `hp_drain_from_hunger_coeff`: 0.05~0.12（建议 0.08）
- `hp_drain_from_energy_coeff`: 0.03~0.08（建议 0.05）
- `hp_drain_cap_per_30m`: 8~16（建议 12）
- `vision_radius_day`: 5~7（建议 6）
- `vision_radius_night`: 2~4（建议 3）
- `torch_light_radius`: 2~4（建议 3）
- `farm_grow_minutes`: 45~90（建议 60）
- `wheat_yield_range`: [1,3]（或固定 2）
- `seed_return_chance`: 10%~30%（建议 20%）

---

## 5) 资源、刷新与随机性下限（避免 KPI 被随机性支配）

### 5.1 资源与物品最小集合

- 原材料：`wood`, `stone`, `seed`, `berry`
- 食物：`berry`, `wheat`
- 建筑对象：`bed`, `box`, `farm_plot`（torch 可选）

### 5.2 资源点类型与刷新

资源点：
- `tree` -> `wood`
- `stone_node` -> `stone`
- `berry_bush` -> `berry`
- `wild_grass` -> 小概率 `seed`（或由保底机制获得）

刷新边界（建议）：
- berry 30~60min
- tree/stone 60~120min
- wild_grass 30~90min

`observe.resources[]` 建议包含：
- `id`（稳定）
- `type`
- `pos`
- `is_depleted`
- `respawn_in_seconds?`（强烈建议；否则 Agent 难以规划等待 vs 迁移）

### 5.3 seed 保底（强烈建议，二选一）

为了确保 72h Gate 不被脸黑卡死：

- 方案 A（推荐）：pity 机制
  - `seed_drop_chance`（10%~25%）
  - `seed_pity_max_fails`（6~10）
  - 建议额外暴露（仅 Safe Zone）：`seed_pity_remaining`（让 Agent 可规划“继续薅草还是换策略”）

- 方案 B（最简单可解释）：一次性 seed_cache 点
  - Safe Zone 出生附近放置一次性 `seed_cache`

---

## 6) 建造与生产（bed/box/farm_plot）

### 6.1 配方（MVP 最小）

- `box`: wood x4
- `farm_plot`: wood x2 + stone x2
- `bed` 两种配方（不增加物品类型，但增加策略多样性）：
  - `bed_rough`: wood x8（保底）
  - `bed_good`: wood x6 + berry x2（sleep 效率更高）

`bed` 在 `observe.objects[]` 暴露：
- `quality: ROUGH | GOOD`

### 6.2 农田循环

- `farm_plant`：消耗 seed x1，farm 进入 `GROWING`
- 生长：`farm_grow_minutes`
- `farm_harvest`：产出 wheat（范围显式） + 概率返种（显式）

---

## 7) 夜晚压力（最终收敛口径）

MVP 夜晚压力：
- **仅**：可见度下降（`vision_radius_night`）

火把（可选增强）：
- torch 覆盖区 `is_lit=true` 并使 tile `is_visible=true`（或至少恢复实体可见）

不引入：
- 夜晚伤害倍率
- 暗骰伤害

---

## 8) 威胁与撤离（无战斗版）

### 8.1 威胁的 MVP 定位

- 威胁用于：
  - 提供 `local_threat_level`（聚合风险）
  - 提供 `threats[]`（可见时）
  - 驱动 Agent 选择 `retreat` 或绕行

不要求：
- 通过“击杀”解决问题（无 combat）。

`observe.threats[]` 最小字段：
- `id`（稳定）
- `type`
- `pos`
- `danger_score`（0..100）

### 8.2 retreat intent

- 语义：向远离最近威胁/高风险方向移动 1~2 格（实现可固定 1 格以简化），并返回动作结果（位置变化等）。

---

## 9) entity id / ref 规则（稳定交互的关键）

### 9.1 稳定 id（硬要求）

`objects/resources/threats` 必须有稳定 `id`：
- 实体存续期间 id 不变
- MVP 当前实现采用坐标派生稳定 id（如 `res_x_y_type` / `thr_x_y` / 持久化 `object_id`），不引入独立生命周期 id 分配器；
  因此“消失后不复用”的生命周期约束在后续版本通过独立实体表补齐。

tile id：可不强制（可由坐标 hash 推导）。

### 9.2 引用方式（ref 规则）

推荐：所有可交互实体动作使用 `*_id`：
- `gather.target_id`
- `sleep.bed_id`
- `farm_plant.farm_id`
- `container_deposit.container_id`

放置类动作使用 `pos`：
- `build.pos`

---

## 10) 动作集（MVP intents）

### 10.1 intents 列表

- `move {direction}` 或 `move {pos}`
- `gather {target_id}`
- `craft {recipe_id, count}`
- `build {object_type, pos}`
- `eat {item_type, count}`
- `rest {rest_minutes (1..120)}`
- `sleep {bed_id}`
- `farm_plant {farm_id}`
- `farm_harvest {farm_id}`
- `container_deposit {container_id, items:[{item_type,count}]}`
- `container_withdraw {container_id, items:[{item_type,count}]}`
- `retreat {}`
- `terminate {}`（中止进行中动作）

### 10.2 action_costs 默认 base_minutes（集中写死，便于 Agent）

建议默认（可调，但必须在 `world.rules` 或 `action_costs` 明确暴露）：
- move = 1
- gather = 5（或 3；二者选一并写死默认）
- craft = 2
- build = 3
- eat = 1
- farm_plant = 2
- farm_harvest = 2
- container_deposit = 1
- container_withdraw = 1
- rest：`base_minutes = 30`（默认预估成本；实际结算按 `rest_minutes`）
- sleep：建议固定 `base_minutes = 60`（避免连续变量导致策略搜索复杂）

> sleep 效率（按 bed quality）建议也在 rules 或 action_costs.sleep.details 中集中暴露。

### 10.3 terminate 语义（MVP 补充约束）

- 用途：中止“进行中动作（ongoing action）”并立即返回可继续决策的状态。
- MVP 仅允许中止：`rest`。
- 若不存在可中止的进行中动作，返回 `REJECTED`（前置条件不满足）。
- 中止会触发结算：
  - `settled_dt_minutes = 已进行分钟数`（按实际已发生时长结算，不按计划剩余时长）。
  - 响应必须返回 `world_time_before_seconds/world_time_after_seconds`，与结算事件一致。
  - 事件中保留 `ongoing_action_ended` 用于复盘（含 planned/actual/forced）。

---

## 11) 错误码层级与可恢复性（统一错误对象）

### 11.1 顶层层级

- `result_code: OK | REJECTED | FAILED`

### 11.2 error 对象（当 result_code != OK 必须返回）

`error: { code, message, retryable, blocked_by, details }`
- `retryable` 总原则：
  - **不改变世界/自身条件直接重试就可能成功** 才为 `true`（例如并发/锁/幂等等临时冲突）
  - `TOO_FAR/BLOCKED/NO_ITEM/CONTAINER_FULL/INVENTORY_FULL` 通常为 `false`

`blocked_by` 最小枚举（不超过 6 个）：
- `NIGHT`
- `NO_LIGHT`
- `INVENTORY_FULL`
- `CONTAINER_FULL`
- `REQUIREMENT_NOT_MET`
- `NOT_VISIBLE`

### 11.3 目标合法性错误码（写死）

- `TARGET_OUT_OF_VIEW`：目标不在 11x11 view 窗口内
- `TARGET_NOT_VISIBLE`：目标在窗口内，但 tile `is_visible=false`

对可导航类错误（最低要求）：
- `BLOCKED/INVALID_TARGET`：返回 `error.details.blocking_tile_pos`
- `TOO_FAR`：返回 `error.details.target_pos`（可选 `distance`）

---

## 12) 事件与复盘（必须可解释、可训练）

### 12.1 最小事件类型

- `action_settled`（每次 action 必出）
- `world_phase_changed`（可选但推荐）
- `game_over`

### 12.2 action_settled 必含字段

- `world_time_before_seconds/world_time_after_seconds/settled_dt_minutes`
- `state_before`（至少 hp/hunger/energy/pos/inventory_used）
- `decision`（intent + params + strategy_hash?）
- `result`（产出/消耗/建造 object_id 等）
- `state_after`

### 12.3 game_over 事件：最后可观测快照（硬要求）

`game_over` 必须包含：
- `death_cause: STARVATION | THREAT | UNKNOWN`（MVP 可先简化三类）
- `state_before_last_action`（固定字段集合）：
  - `hp/hunger/energy/position/inventory_used/world_time_seconds`
  - `inventory_summary`：建议固定结构 `{ total_items:int, top:[{item_type,count}...] }`
- `state_after_last_action`（同结构）
- `last_safe_home`：`home` 或 null
- `last_known_threat`：若有可见威胁：`{id,type,pos,danger_score}` 否则 null

---

## 13) 72h Gate：最坏情况边界（验收与调参用）

### 13.1 Gate 最小物资清单（保底路径）

- box：wood 4
- farm_plot：wood 2 + stone 2
- bed_rough：wood 8
- 首次播种：seed 1

合计（保底）：
- wood 14
- stone 2
- seed 1

### 13.2 可行性边界（避免白天被卡夜晚）

定义：home 到最近资源点距离 `d`（曼哈顿）。
最低可行约束（建议调参检查项）：
- `2*d*move_minutes + gather_minutes <= day_length_minutes`

推荐默认：`move_minutes=1`、`gather_minutes=5`、`day_length_minutes=10`，Safe Zone 内尽量保证 `d<=2`。

---

## 14) 最小 JSON 示例（MVP）

> 仅为“形状与语义”示例；字段命名以工程实现为准，但语义必须覆盖。

### 14.1 observe 响应示例（最小）

```json
{
  "view": {"width": 11, "height": 11, "center": {"x": 3, "y": -1}, "radius": 5},
  "agent_state": {
    "hp": 72,
    "hunger": 40,
    "energy": 18,
    "status_effects": ["EXHAUSTED"],
    "position": {"x": 3, "y": -1},
    "inventory_capacity": 30,
    "inventory_used": 12,
    "inventory": {"wood": 6, "berry": 2},
    "home": {"x": 0, "y": 0, "radius": 6},
    "session_id": "s_01HXYZ..."
  },
  "world": {
    "world_time_seconds": 123456,
    "time_of_day": "night",
    "next_phase_in_seconds": 120,
    "rules": {
      "standard_tick_minutes": 30,
      "drains_per_30m": {
        "hunger_drain": 4,
        "energy_drain": 0,
        "hp_drain_model": "dynamic_capped",
        "hp_drain_from_hunger_coeff": 0.08,
        "hp_drain_from_energy_coeff": 0.05,
        "hp_drain_cap": 12
      },
      "thresholds": {"critical_hp": 15, "low_energy": 20},
      "visibility": {"vision_radius_day": 6, "vision_radius_night": 3, "torch_light_radius": 3},
      "seed": {"seed_drop_chance": 0.2, "seed_pity_max_fails": 8, "seed_pity_remaining": 3},
      "farming": {"farm_grow_minutes": 60, "wheat_yield_range": [1, 3], "seed_return_chance": 0.2}
    }
  },
  "action_costs": {
    "move": {"base_minutes": 1, "delta_hunger": 0, "delta_energy": -1, "requirements": []},
    "gather": {"base_minutes": 5, "delta_hunger": 0, "delta_energy": -2, "requirements": ["ADJACENT"]},
    "rest": {"base_minutes": 30, "delta_hunger": 0, "delta_energy": 12, "requirements": []},
    "sleep": {"base_minutes": 60, "delta_hunger": 0, "delta_energy": 18, "requirements": ["ON_BED"]}
  },
  "tiles": [
    {"pos": {"x": 3, "y": -1}, "terrain_type": "grass", "is_walkable": true, "is_lit": false, "is_visible": true},
    {"pos": {"x": 3, "y": -2}, "terrain_type": "grass", "is_walkable": true, "is_lit": false, "is_visible": true}
  ],
  "objects": [
    {"id": "obj_box_001", "type": "box", "pos": {"x": 0, "y": 0}, "capacity_slots": 60, "used_slots": 10},
    {"id": "obj_bed_001", "type": "bed", "quality": "ROUGH", "pos": {"x": 0, "y": 1}},
    {"id": "obj_farm_001", "type": "farm_plot", "pos": {"x": 1, "y": 0}, "state": "GROWING"}
  ],
  "resources": [
    {"id": "res_berry_01", "type": "berry_bush", "pos": {"x": 3, "y": -2}, "is_depleted": false}
  ],
  "threats": [],
  "local_threat_level": 0
}
```

### 14.2 action 响应示例（失败：TARGET_NOT_VISIBLE）

```json
{
  "result_code": "REJECTED",
  "settled_dt_minutes": 0,
  "world_time_before_seconds": 123460,
  "world_time_after_seconds": 123460,
  "error": {
    "code": "TARGET_NOT_VISIBLE",
    "message": "target is in view window but not visible",
    "retryable": false,
    "blocked_by": ["NOT_VISIBLE"],
    "details": {"in_window": true, "is_visible": false}
  },
  "updated_state": null,
  "events": []
}
```

### 14.3 action 响应示例（成功：container_deposit）

```json
{
  "result_code": "OK",
  "settled_dt_minutes": 30,
  "world_time_before_seconds": 123460,
  "world_time_after_seconds": 125260,
  "updated_state": {
    "hp": 72,
    "hunger": 35,
    "energy": 16,
    "position": {"x": 0, "y": 0},
    "inventory_capacity": 30,
    "inventory_used": 8,
    "inventory": {"wood": 2, "berry": 2},
    "status_effects": []
  },
  "events": [
    {
      "event_type": "action_settled",
      "occurred_at": "2026-02-17T10:00:00Z",
      "session_id": "s_01HXYZ...",
      "world_time_before_seconds": 123460,
      "world_time_after_seconds": 125260,
      "settled_dt_minutes": 30,
      "decision": {"intent": {"type": "container_deposit", "params": {"container_id": "obj_box_001", "items": [{"item_type": "wood", "count": 4}]}}},
      "result": {"deposited": [{"item_type": "wood", "count": 4}]},
      "state_before": {"hp": 72, "hunger": 40, "energy": 18, "pos": {"x": 0, "y": 0}, "inventory_used": 12},
      "state_after": {"hp": 72, "hunger": 35, "energy": 16, "pos": {"x": 0, "y": 0}, "inventory_used": 8}
    }
  ]
}
```

---

## 15) 变更说明

- v1.0：将 v0.1~v0.9 全部内容合并成一个**完整文档**；按最终拍板移除 combat；将空间/可见性/错误码/参数显式暴露/随机性下限/存取动作/复盘事件等契约集中、写死。
