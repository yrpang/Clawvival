# Clawvival 前端控制台方案（React）

## 1. 目标与范围

目标：提供一个可运维、可观测、可回放的 Agent 控制台，支持用户实时查看地图、Agent 状态、位置、动作历史与动作前后效果。

本期范围（MVP）：
- 实时地图与世界基础信息（昼夜、世界时间、威胁）。
- Agent 当前状态（Vitals、Inventory、位置、ongoing_action、cooldowns）。
- 历史动作分页列表（每页 20 条）+ 同栏展开详情。
- 关键事件回放信息（action_settled / ongoing_action_ended / game_over 等）。

非本期范围：
- 地图编辑器。
- 多人协作与权限系统。
- 复杂 BI 报表。

## 2. 信息架构与页面布局

页面采用三栏布局（Desktop），移动端改为 Tab 切换。

### 2.1 顶部全局栏（Header）
- 当前 Agent 标识（agent_id / session_id）。
- world_time_seconds。
- time_of_day（day/night）。
- next_phase_in_seconds。
- 实时连接状态（live / reconnecting / offline）。

### 2.2 左栏：Agent 状态面板
- Vitals：HP、Hunger、Energy（带阈值色彩）。
- 位置：x/y、current_zone。
- Inventory 摘要（可折叠查看完整）。
- Ongoing Action：类型、开始/结束、剩余时长。
- Action Cooldowns（按 action_type 展示 remaining seconds）。

### 2.3 中栏：地图主视图
- 11x11 可视窗口（与后端 observe 视窗一致）。
- 显示：tile、resource、object、agent 当前坐标。
- 高亮：当前位置、可交互资源点、威胁高点。
- 支持：缩放（仅视觉）、悬浮 tooltip（tile 信息）。

### 2.4 右栏：历史面板（列表 + 展开详情，合并设计）
- 顶部：筛选区（action_type、result_code、时间范围）。
- 中部：历史列表（每页 20 条）。
- 行内字段：时间、动作类型、结果、world_time_before/after、vitals 关键 delta。
- 点击行后在同栏展开详情（Accordion）：
  - intent + params
  - state_before / state_after
  - vitals_delta、inventory_delta
  - vitals_change_reasons
  - 关联事件列表（按 occurred_at 排序）

## 3. 核心交互流程

### 3.1 实时刷新
- 首屏并行拉取：`/status` + `/observe` + `/replay?page=1&page_size=20`。
- 轮询策略（MVP）：
  - status/observe：2~5s（可配置）。
  - replay：10~15s 或用户切页时主动刷新。
- 若后端提供 SSE/WS，替换为推送驱动并保留轮询兜底。

### 3.2 历史详情展开
- 用户点击某条记录 -> 右栏展开对应详情。
- 二次点击可收起；切页后默认不保留展开项（减少状态复杂度）。
- 提供“定位到动作发生位置”按钮（在地图上短暂高亮）。

### 3.3 异常与空态
- API 错误：统一错误条 + 重试按钮。
- 空历史：提示暂无动作记录。
- 断线：标记 offline，并自动指数退避重连。

## 4. 技术栈建议

## 4.1 基础栈
- React 18 + TypeScript + Vite
- React Router
- TanStack Query（服务端状态、分页、轮询、缓存）
- Zustand（轻量 UI 状态：筛选项、选中历史、地图临时高亮）

### 4.2 UI 与可视化
- UI 组件：MUI（优先，成熟、数据密集场景更稳）
- 地图渲染：MVP 使用 CSS Grid；后续视性能迁移 Canvas/WebGL
- 图形（可选）：Recharts（Vitals 趋势）

### 4.3 数据与质量
- 网络层：Axios + 统一 API Client
- 运行时契约校验：Zod（关键响应）
- 测试：Vitest + React Testing Library + Playwright（关键流程）
- 规范：ESLint + Prettier

## 5. 前后端接口映射（结合当前后端）

已存在接口（按当前项目语义）：
- `GET /api/agent/status`
- `GET /api/agent/observe`
- `GET /api/agent/replay`

前端建议请求形态：
- status：`agent_id`
- observe：`agent_id`
- replay：`agent_id`, `page`, `page_size=20`, `action_type?`, `from?`, `to?`

若 replay 目前不支持分页参数，建议后端补充：
- `page`（1-based）
- `page_size`（固定 20，允许前端传参便于复用）
- 返回 `total`, `page`, `page_size`, `items`

## 6. 前端数据模型（ViewModel）

```ts
export type AgentStatusVM = {
  agentId: string;
  sessionId: string;
  vitals: { hp: number; hunger: number; energy: number };
  position: { x: number; y: number };
  currentZone: string;
  inventory: Record<string, number>;
  ongoingAction: null | {
    type: string;
    startAt?: string;
    endAt: string;
    minutes: number;
  };
  cooldowns: Record<string, number>;
};

export type WorldSnapshotVM = {
  worldTimeSeconds: number;
  timeOfDay: "day" | "night";
  nextPhaseInSeconds: number;
  threatLevel: number;
  tiles: Array<{
    x: number;
    y: number;
    kind: string;
    zone: string;
    passable: boolean;
    resource?: string;
    baseThreat?: number;
  }>;
};

export type ActionHistoryItemVM = {
  executionId: string;
  occurredAt: string;
  actionType: string;
  resultCode: string;
  worldTimeBeforeSeconds?: number;
  worldTimeAfterSeconds?: number;
  stateBefore?: Record<string, unknown>;
  stateAfter?: Record<string, unknown>;
  result?: Record<string, unknown>;
  events: Array<{ type: string; occurredAt: string; payload?: Record<string, unknown> }>;
};
```

## 7. 建议目录结构（前端代码）

当前仓库是后端为主，无前端目录。建议新增 `apps/web`：

```text
apps/web/
  src/
    app/
      router.tsx
      providers.tsx
    pages/
      dashboard/
        DashboardPage.tsx
    widgets/
      header/
      agent-panel/
      map-view/
      history-panel/
    features/
      history-filter/
      replay-pagination/
    entities/
      agent/
      world/
      action/
    shared/
      api/
      ui/
      lib/
      config/
```

说明：
- `apps/web` 比 `web/` 更适合后续扩展（例如 `apps/admin`）。
- 文档继续放 `docs/frontend/`，代码放 `apps/web/`，职责清晰。

## 8. 性能与可用性要求

- 历史列表渲染目标：单页 20 条，无感卡顿。
- 地图刷新目标：2~5s 轮询下 UI 稳定（避免整页重绘）。
- 关键策略：
  - Query 分层缓存（status/observe/replay 分离）。
  - 对历史详情使用惰性展开。
  - map tile 使用稳定 key，减少 diff 开销。

## 9. 安全与边界

- 前端不存储敏感密钥到 localStorage（如 agent_key）。
- 仅通过请求头发送凭据（若有）。
- 所有用户可见字段以 API 返回为准，不做“推断式写回”。

## 10. 迭代计划

### 阶段 A（2~3 天）
- 初始化 `apps/web`。
- 打通 status/observe 数据链路。
- 完成三栏骨架 + 地图基础渲染。

### 阶段 B（2~3 天）
- 接入 replay 分页（20/页）。
- 右栏列表 + 展开详情。
- 关键筛选与错误处理。

### 阶段 C（1~2 天）
- 轮询与连接状态治理。
- E2E 测试（加载、翻页、展开详情）。
- UI 细节收敛与性能优化。

## 11. 落地位置建议（本仓库）

- 方案文档：`docs/frontend/agent-console-ui-plan.md`（当前文件）
- 后续 API 对齐文档：`docs/frontend/api-contract-notes.md`
- 前端工程目录：`apps/web`

这个放置方式与现有 `docs/design`、`docs/engineering` 并列，不会污染后端实现目录，后续协作成本最低。
