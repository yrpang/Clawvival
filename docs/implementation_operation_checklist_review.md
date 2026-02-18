# Clawvival 当前实现操作清单（审查版）

> 目标：基于当前代码实现，给出可审查的操作清单，明确每项操作的前置条件、操作影响、失败语义、可中断性。
>
> 基线代码范围：`internal/adapter/http/*`、`internal/app/*`、`internal/domain/survival/*`、`internal/adapter/world/runtime/*`。

## 1. 系统级前置条件

- 鉴权
  - `POST /api/agent/register` 免鉴权。
  - 其他 `/api/agent/*` 必须携带请求头：`X-Agent-ID` + `X-Agent-Key`。
  - 缺失或错误分别返回：`missing_agent_credentials` / `missing_agent_id` / `missing_agent_key` / `invalid_agent_credentials`。
- Agent 初始化
  - 先调用注册接口拿到 `agent_id` 与 `agent_key`。
  - 注册时会落初始状态：`HP=100, Hunger=80, Energy=60, Pos=(0,0), InventoryCapacity=30`。
- 时间结算
  - `dt` 由服务端计算，客户端请求体出现 `dt` 会被拒绝（`dt_managed_by_server`）。
  - 常规结算 `dt` 来源：距离最近一次 `action_settled` 的分钟数，范围被钳制到 `[1,120]`，首次默认 `30`。
- 幂等
  - `POST /api/agent/action` 必须带 `idempotency_key`。
  - 同 `agent_id + idempotency_key` 重放会返回首次已落库结果，不会重复结算。

## 2. API 操作清单

## 2.1 `POST /api/agent/register`

- 前置条件
  - 无鉴权。
- 操作影响
  - 新增 `agent_credentials` 与 `agent_states`。
  - 返回明文 `agent_key`（仅此处返回）。
- 可中断性
  - 不适用（同步一次性操作）。

## 2.2 `POST /api/agent/observe`

- 前置条件
  - 鉴权通过。
- 操作影响
  - 只读，不写状态。
  - 返回 11x11 固定窗口（radius=5）、`tiles/objects/resources/threats`、`world.rules`、`action_costs`、`hp_drain_feedback`。
- 可中断性
  - 不适用（只读）。

## 2.3 `POST /api/agent/status`

- 前置条件
  - 鉴权通过。
- 操作影响
  - 只读，不写状态。
  - 返回 `agent_state` + 世界时间/规则/动作成本。
- 可中断性
  - 不适用（只读）。

## 2.4 `GET /api/agent/replay`

- 前置条件
  - 鉴权通过。
- 操作影响
  - 只读；按 `limit/occurred_from/occurred_to/session_id` 过滤事件。
  - 返回 `events` 与基于事件重建的 `latest_state`（仅使用事件 `state_after`）。
- 可中断性
  - 不适用（只读）。

## 2.5 `POST /api/agent/action`

- 前置条件
  - 鉴权通过。
  - `idempotency_key` 必填。
  - `intent.type` 必须在支持集合内。
  - 请求体不得含 `dt` 字段。
- 操作影响
  - 唯一写入口：可能更新 `agent_states / world_objects / domain_events / action_executions / agent_sessions`。
  - 成功返回 `settled_dt_minutes`、`world_time_before_seconds`、`world_time_after_seconds`、`events`。
  - 失败时对动作类错误统一返回 `result_code=REJECTED` 与 `action_error`。
- 可中断性
  - 仅 ongoing 的 `rest` 支持被 `terminate` 中断。

## 2.6 Skills 与运维接口

- `GET /skills/index.json`、`GET /skills/*filepath`
  - 只读静态分发，无游戏状态写入。
- `GET /ops/kpi`
  - 只读；未配置 provider 返回 `not_configured`。

## 3. Action Intent 审查清单

## 3.1 统一前置条件（所有 action）

- 若存在 ongoing action：
  - 非 `terminate` 请求会被拒绝（`action_in_progress`），除非该 ongoing 已到结束时间并在本次请求开始时被自动结算。
- 冷却（仅部分动作）
  - `build=5m`、`craft=5m`、`farm_plant=3m`、`move=1m`。
- 可能的通用拒绝码
  - `action_precondition_failed`、`action_invalid_position`、`action_cooldown_active`、`TARGET_OUT_OF_VIEW`、`TARGET_NOT_VISIBLE`、`INVENTORY_FULL`、`CONTAINER_FULL`。

## 3.2 分动作明细

| intent | 前置条件 | 主要影响 | 可中断性 |
|---|---|---|---|
| `gather` | `target_id` 必填；需可解析 `res_x_y_type`；目标在窗口半径 5 内；夜间需满足可见半径；目标 tile 可见且资源类型匹配 | 结算 vitals；按快照资源增加背包；触发 seed pity 逻辑（失败累计到阈值补 1 seed） | 否（同步结算） |
| `rest` | `rest_minutes` 必须在 `[1,120]` | 不立即结算生命值；写入 `ongoing_action` 并记录 `rest_started`；本次 `dt=0` | 是（仅此动作可被 `terminate` 打断） |
| `sleep` | `bed_id` 必填；对象存在且是 bed；床坐标必须与当前坐标一致 | 进入正常结算：能量/HP 恢复，并写 `action_settled` | 否 |
| `move` | 有位移参数；步长不能超过 1 格；目标 tile 必须存在且可通行 | 更新坐标并扣除体力/饥饿；受 1 分钟冷却 | 否 |
| `build` | `object_type` 支持且 `pos` 必填；资源足够 | 扣材料；写 `build_completed` 事件；落 `world_objects`（bed/box/farm 等） | 否 |
| `farm_plant` | `farm_id` 必填；有 seed；对象是 farm 且状态 `IDLE` | 消耗 seed；farm 状态写为 `GROWING`，写入 ready 时间 | 否 |
| `farm_harvest` | `farm_id` 必填；对象是 farm 且已 ready（或 `ready_at` 到时） | 增加 wheat（并可能返 seed）；farm 状态回 `IDLE` | 否 |
| `container_deposit` | `container_id` + `items` 必填；对象是 box；背包库存足够；箱子容量足够 | 背包扣减；箱子 `object_state.inventory` 与 `used_slots` 增加 | 否 |
| `container_withdraw` | `container_id` + `items` 必填；对象是 box；箱内库存足够；背包容量足够 | 背包增加；箱子库存和 `used_slots` 减少 | 否 |
| `retreat` | 无强制参数 | 结算前会尝试自动计算“远离最高威胁”的一步位移 | 否 |
| `craft` | `recipe_id > 0` 且材料满足配方 | 目前支持 `plank(wood*2)`、`bread(wheat*2)` | 否 |
| `eat` | `item_type` 仅 `berry/bread/wheat`；`count > 0`；库存足够 | 逐个消耗食物并恢复饥饿值 | 否 |
| `terminate` | 必须存在 ongoing action；且 ongoing 类型可中断（当前仅 `rest`） | 强制完成 ongoing 结算，清除 ongoing，写 `ongoing_action_ended(forced=true)` | 不适用（它本身是中断操作） |

## 4. 数据影响矩阵

- `agent_states`
  - 几乎所有成功 action 都会更新（包括 version、vitals、position、inventory、ongoing_action）。
  - `rest` 启动时不做生命结算，但会更新 version 与 ongoing。
- `action_executions`
  - 每次 action 都写入（含 `terminate`、`rest`），承载幂等回放依据。
- `domain_events`
  - 结算动作会有 `action_settled`；特殊情况下追加 `game_over`、`critical_hp`、`force_retreat`、`world_phase_changed`、`seed_pity_triggered`、`ongoing_action_ended` 等。
- `world_objects`
  - `build` 创建对象。
  - `farm_*` / `container_*` 更新对象状态。
- `agent_sessions`
  - action 前确保 session active。
  - 结果为 `GameOver` 时关闭 session。

## 5. 失败与阻断语义（审查重点）

- 动作类拒绝统一返回
  - `result_code=REJECTED`
  - `settled_dt_minutes=0`
  - `world_time_before_seconds=0`
  - `world_time_after_seconds=0`
  - `error` 与 `action_error` 双字段镜像。
- 阻断原因（`blocked_by`）
  - 常见值：`REQUIREMENT_NOT_MET`、`NOT_VISIBLE`、`INVENTORY_FULL`、`CONTAINER_FULL`。
- 非动作类错误（如鉴权失败、资源不存在）
  - 走通用 `error.code` 返回，不带 `result_code=REJECTED`。

## 6. 审查用核对项（Checklist）

- 接口层
  - [ ] 除 `register` 外，是否全部强制 `X-Agent-ID/X-Agent-Key`。
  - [ ] `action` 是否严格拒绝客户端 `dt`。
  - [ ] `action` 拒绝响应结构是否保持 `REJECTED + action_error` 兼容。
- 业务层
  - [ ] `ActionUseCase` 是否仍为唯一写入口。
  - [ ] 幂等是否以 `agent_id + idempotency_key` 实现且可回放世界时间窗。
  - [ ] ongoing 可中断范围是否仍仅 `rest`。
  - [ ] `terminate` 是否不会中断非 interruptible 动作。
- 数据层
  - [ ] `action_executions`、`domain_events` 是否与 state 更新同事务。
  - [ ] `world_objects` 状态更新是否与动作结算一致（箱子容量、农田状态）。
  - [ ] `GameOver` 时 session 关闭是否可靠。

## 7. 代码锚点（便于审查追溯）

- 路由与错误映射：`internal/adapter/http/handler.go`
- 动作编排主流程：`internal/app/action/usecase.go`
- 结算规则：`internal/domain/survival/service_settlement.go`
- 生存配方/建造/食物定义：`internal/domain/survival/production.go`
- 观察视图与规则透出：`internal/app/observe/usecase.go`
- 状态透出：`internal/app/status/usecase.go`
- 状态派生（effects/capacity）：`internal/app/stateview/derive.go`
- 世界快照与昼夜/区块：`internal/adapter/world/runtime/provider.go`
- 注册与鉴权：`internal/app/auth/usecase.go`

