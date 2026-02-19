# Clawvival Web 重构计划（TailwindCSS）

## 1. 背景与目标

当前 `apps/web` 页面功能完整，但样式与业务逻辑耦合较高（单体 `App.tsx` + 大体量 `index.css`），在持续迭代地图交互、历史浮窗、主题适配时维护成本较高。

本次重构目标：

1. 架构清晰：按业务模块拆分，降低改动影响面。
2. 体验稳定：功能不回退，响应式、可读性、交互一致。
3. 工程可持续：统一数据层、样式体系、测试与发布流程。

---

## 2. 重构原则

1. **功能等价优先**：先迁移结构与样式体系，再做交互增强。
2. **渐进式替换**：允许 Tailwind 与旧 CSS 并行，避免一次性大改。
3. **模块边界清晰**：容器负责编排，组件负责展示。
4. **可验证交付**：每阶段都有明确验收项与回归检查。
5. **并行期样式边界**：Tailwind 仅用于新迁移组件；旧 `index.css` 在并行期只做缺陷修复，不新增业务视觉逻辑。

---

## 3. 目标目录结构（建议）

```text
apps/web/src
├─ app/
│  ├─ providers/
│  ├─ router/
│  └─ config/
├─ features/
│  ├─ topbar/
│  ├─ agent-state/
│  ├─ map/
│  └─ history/
├─ entities/
│  ├─ agent/
│  ├─ world/
│  └─ action/
├─ shared/
│  ├─ api/
│  ├─ ui/
│  ├─ hooks/
│  ├─ lib/
│  └─ styles/
└─ main.tsx
```

---

## 4. 分阶段实施计划

## 阶段 0：基线冻结（0.5 天）

目标：建立“可对比”的当前基线。

任务：
- 记录关键页面截图（白天/夜晚、地图选中、历史选中、浮窗展开）。
- 记录关键交互清单：
  - 地图选中/取消
  - 历史选中与地图联动
  - Tile/Action 浮窗定位
  - 1 分钟自动刷新 + 手动刷新
- 明确回归命令：`npm run lint && npm run test && npm run build`。

验收：
- 基线资料齐全，可用于重构前后对照。

## 阶段 1：基础设施（0.5~1 天）

目标：接入 Tailwind，不改变页面行为。

任务：
- 安装并配置 TailwindCSS（Vite）。
- 建立全局 token（颜色、字号、间距、阴影、层级）。
- 保留旧 `index.css`，采用“新组件用 Tailwind，旧页面暂保留”的并行策略。

验收：
- Tailwind 类可在页面生效。
- 现有构建与页面可正常运行。

## 阶段 2：架构拆分（1~1.5 天）

目标：拆分 `App.tsx`，按功能模块归位。

任务：
- 提取 feature 容器：`TopbarPanel`、`AgentStatePanel`、`MapPanel`、`HistoryPanel`。
- 抽离共享逻辑到 hooks：
  - 轮询刷新与 `lastRefresh`
  - map selection / history selection
  - overlay 位置计算
- 统一类型入口（`entities/*` + `shared/api/*`）。

验收：
- `App.tsx` 仅保留页面级编排。
- 功能与当前一致，无明显视觉倒退。

## 阶段 3：数据层标准化（1 天）

目标：统一 React Query 使用方式与数据转换。

任务：
- 统一 query key 与 fetch hooks（`useAgentStatus` / `useAgentObserve` / `useAgentReplay`）。
- 统一错误、空态、加载态组件。
- 抽离 DTO -> ViewModel（避免组件里散落 `Map`/`filter`/`format` 逻辑）。

验收：
- 页面组件不直接拼装底层 API 数据。
- 数据状态管理路径清晰、可复用。

## 阶段 4：UI 迁移到 Tailwind（1.5~2 天）

目标：完成主要页面 UI 的 Tailwind 化。

任务：
- 先迁移基础原子组件：`Button/Input/Badge/Card/Panel`。
- 再迁移业务区块：
  - Topbar
  - History 卡片与 Action Detail 浮窗
  - Map 网格与 Tile Detail 浮窗
- 保持夜间/白天主题显著差异。

验收：
- 视觉与交互达到当前水平或更好。
- 不再依赖大段全局样式实现核心布局。

## 阶段 5：可访问性与性能（1 天）

目标：提升长期稳定性与可用性。

任务：
- 键盘操作与焦点态（按钮、列表项、浮窗）。
- 性能优化：
  - 重计算 memo 化
  - 必要时历史列表虚拟化
  - 减少不必要 re-render
- tooltip/title 与截断信息一致（例如时间区间、Object 省略号）。
- 性能预算（验收阈值）：
  - 首屏 JS gzip 体积目标：`<= 180 KB`（当前约 76 KB，可持续跟踪）
  - 关键交互响应（选中历史 -> 地图高亮）：`<= 100ms`
  - 历史列表滚动：常见机器保持接近 60 FPS，无明显掉帧卡顿

验收：
- 交互无明显卡顿。
- 可访问性基本要求满足。

## 阶段 6：测试与发布收口（0.5~1 天）

目标：确保重构可发布。

任务：
- 补齐关键单测：
  - history 时间展示规则
  - tile/detail overlay 定位逻辑
  - object 多实例展示规则
- 跑完整校验并修复回归。
- 发布到 GitHub Pages 验证线上可用。

验收：
- `lint/test/build` 全绿。
- 线上页面功能与配置正确（`clawvival.app` + `api.clawvival.app`）。

---

## 5. 里程碑与交付物

- **M1（基础可运行）**：阶段 1~2 完成。
  - 交付：Tailwind 接入 + 模块化架构初步落地。
- **M2（核心体验迁移）**：阶段 3~4 完成。
  - 交付：Map/History/Overlay 全部迁移到新范式。
- **M3（可发布收口）**：阶段 5~6 完成。
  - 交付：测试完善、性能优化、线上验证通过。

---

## 6. 风险与应对

1. **样式回归风险**
- 应对：分模块迁移；保留旧样式回退窗口；截图对照验收。

2. **交互行为偏差（尤其浮窗/地图联动）**
- 应对：将定位与联动逻辑抽成纯函数 + 单测。

3. **迭代中断导致“半旧半新”难维护**
- 应对：每阶段都可独立合并，避免大长分支。

---

## 7. 验收清单（发布前）

- [ ] 地图完整显示，选中态可取消。
- [ ] Tile Detail 为浮窗，四角避让逻辑正常。
- [ ] History 选中高亮清晰，Action Detail 浮窗正常。
- [ ] Object 多实例：地图省略显示、详情完整展示。
- [ ] 顶栏在窄宽下不溢出，输入框可压缩。
- [ ] 日夜主题文字可读性通过。
- [ ] `npm run lint` 通过。
- [ ] `npm run test` 通过。
- [ ] `npm run build` 通过。
- [ ] 线上 API 连通性验证通过（`https://api.clawvival.app` 可访问）。
- [ ] 前端到 API 的 CORS 验证通过（浏览器环境 `observe/status/replay` 正常）。
- [ ] GitHub Pages 发布成功。

---

## 8. 下一步（建议执行顺序）

1. 先执行阶段 1（Tailwind 接入，不改 UI）。
2. 再执行阶段 2（拆分 `App.tsx` 与 hooks）。
3. 最后进入阶段 3~4 的 UI 迁移。
