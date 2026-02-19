# Action UseCase Skeleton (ActionSpec + Engine)

## Purpose

This document captures the agreed skeleton for refactoring `internal/app/action/usecase.go` into a clearer, stable orchestration model.

Goals:
- Keep one fixed execution pipeline in app layer.
- Move action-specific differences into `ActionSpec` handlers.
- Support both normal settlement and early-complete actions (for example, `rest` start).

## Scope and Boundaries

- Layer: `internal/app/action` (application orchestration).
- Domain settlement logic still belongs to `internal/domain/survival`.
- Adapter-specific persistence/runtime details remain in adapter implementations behind ports.

## Core Types

### ActionMode

`ActionMode` defines high-level lifecycle behavior of an action type.

- `ActionModeSettle`: normal action, settled in current request.
- `ActionModeStartOngoing`: starts an ongoing action (for example, rest timer).
- `ActionModeFinalizeOnly`: only finalizes current ongoing action (for example, terminate).

### ExecuteMode

`ExecuteMode` is returned by handler `Execute` stage.

- `ExecuteModeContinue`: continue pipeline.
- `ExecuteModeCompleted`: execution intent is complete; engine still persists shared invariants before returning.

### ActionSpec

`ActionSpec` declares behavior for one action type.

Fields:
- `Type survival.ActionType`: action identity.
- `Mode ActionMode`: lifecycle style.
- `CanTerminate bool`: whether this ongoing action is interruptible by terminate.
- `Handler ActionHandler`: action-specific hooks.

### ActionHandler

`ActionHandler` contains optional hooks for action-specific logic:

- `BuildContext(ctx, uc, ac) error`
- `Precheck(ctx, uc, ac) error`
- `ExecuteActionAndPlan(ctx, uc, ac) (ExecuteMode, error)`

A `BaseHandler` should provide no-op defaults for all hooks.

Ownership rule:
- Handler hooks prepare/derive action-specific data and side-effect plan.
- Engine owns final write orchestration for shared invariants (`state`, `action_execution`, `events`, session close, metrics).
- Do not write the same persistence concern in both handler and engine.
- Project decision: use a single strategy where Engine performs all persistence in `PersistAndRespond`.
- Single-hook decision: keep one execution hook `ExecuteActionAndPlan`; do not introduce a second plan hook.

### ActionContext

`ActionContext` is the shared per-request working state, split by mutability and ownership.

Suggested structure:
- `In` (immutable input):
  - `Req`, `NowAt`, `AgentID`
- `View` (read-side context):
  - `Spec`, `StateBefore`, `Snapshot`, `EventsBefore`, `PreparedObj`
- `Plan` (write-side source of truth):
  - `StateToSave`, `EventsToAppend`, `ExecutionToSave`, `ObjectOps`, `ResourceOps`, `CloseSession`
- `Tmp` (ephemeral working vars):
  - `ResolvedIntent`, `DeltaMinutes`, `SettleResult`, flags

Rules:
- `PersistAndRespond` reads only `Plan` for writes.
- Handler must not call repositories directly.
- Temporary fields under `Tmp` must not be persisted unless copied into `Plan`.
- `ExecuteActionAndPlan` must produce a complete `Plan` for the chosen action path (including early-complete paths).

### ActionEngine

`ActionEngine` owns:
- `Registry map[survival.ActionType]ActionSpec`
- Existing ports/services (`TxManager`, repos, `World`, `Settle`, `Metrics`, `Now`)

It runs one fixed pipeline and delegates action differences to specs.
Handlers receive `UseCase` dependency bundle (`uc`) explicitly, so action-specific logic can access required services without breaking layer boundaries.

## Fixed Pipeline (8 Steps)

1. `ValidateRequest`
- Normalize and validate request/action params.

2. `ReplayIdempotent`
- Return previous execution if idempotency key already exists.

3. `LoadStateAndFinalizeOngoing`
- Load state, finalize existing ongoing action if needed.
- For terminate flow, enforce interruptibility via ongoing-action spec metadata.
- Note: this step resolves spec by `state.OngoingAction.Type` (if present), not by request action type.

4. `ResolveSpec`
- Lookup `ActionSpec` by action type.

5. `BuildContext`
- Prepare shared context needed by this action.

6. `RunPrechecks`
- Action-specific preconditions (resource/object/visibility/cooldown/position).

7. `ExecuteActionAndPlan`
- Perform action execution.
- May return `ExecuteModeCompleted` for early-complete actions.
- Populate write intent into `ActionContext.Plan`.

8. `PersistAndRespond`
- Persist state/events/execution/object/resource side effects.
- Build response and return.

## Persistence Order Contract

To preserve existing behavior, `PersistAndRespond` should keep a deterministic write order.

Recommended order (normal settled path):
1. Save updated state with version check.
2. Stamp event payload shared fields (`agent_id`, `session_id`, optional `strategy_hash`).
3. Save `action_execution` record (for idempotent replay).
4. Persist object/resource side effects (for example box/farm/gather depletion/build object creation).
5. Append domain events.
6. Close session if result is game over.
7. Build response (`world_time_before/after`, settlement summary, result code).

For ongoing-finalization path, keep equivalent semantics and ordering guarantees.

## Early-Complete Semantics

Early-complete actions are first-class behavior via `ExecuteModeCompleted`, not hardcoded branch clutter in the engine.

Rules:
- If handler returns `ExecuteModeCompleted`, engine marks completed mode and still runs `PersistAndRespond`.
- Engine-level invariants (validation, idempotency, ongoing-finalization) still run before this point.
- `ExecuteModeCompleted` must preserve idempotency and observability invariants:
  - action execution must be persisted exactly once;
  - required state/event writes must be completed;
  - metrics outcome must still be recorded by engine.

Implementation guidance:
- Early-complete handlers must not persist shared invariants directly.
- Handler marks completion intent/data in `ActionContext` (primarily `Plan` and response data under `Tmp`); engine still executes `PersistAndRespond`.
- Engine then returns the completed response after shared persistence is done.

## Ongoing Finalization Strategy

`LoadStateAndFinalizeOngoing` should not embed all action business logic.

It should:
- Read current ongoing type from state.
- Resolve ongoing metadata from registry (`Mode`, `CanTerminate`).
- Decide finalize/skip/reject by lifecycle rules.
- Delegate actual settlement to existing domain settlement service.

This keeps ongoing lifecycle orchestration in app layer while preserving domain ownership of settlement rules.

## Registration Pattern

Action behaviors are defined by registry entries.

When adding a new action:
- Add one `ActionSpec` registration.
- Implement only required hooks; reuse no-op defaults otherwise.
- Avoid changing engine pipeline order.

Expected effect:
- New action changes remain localized.
- Engine complexity stays stable as action count grows.

## Handler File Organization Rules

Current agreement: organize by **business capability cluster**, not by “one action per file” and not by arbitrary file size.

Hard rules:
- `1 file = 1 capability cluster`.
- File name must reflect capability: `action_<capability>_logic.go`.
- Multiple actions may share one file only when they share the same helper set and port dependencies.
- If actions do not share precheck/execute helpers, split into separate files.
- Cross-action generic logic must stay in shared files (`action_settle_logic.go`, `action_precheck_runtime.go`, etc.), not in any single action file.
- Each action file should keep: action-specific precheck, action-specific execute, and only the helpers needed by those actions.

Decision heuristic (for future changes):
1. If adding a new action requires introducing mostly new helpers, create a new capability file.
2. If adding a new action reuses an existing capability helper chain, co-locate in that capability file.
3. If one file starts to mix unrelated concepts (for example movement + crafting), split immediately.

## Current Capability Mapping (Target State)

- `action_gather_logic.go`: `gather` action-specific precheck and execution extras (visibility/depletion/seed pity).
- `action_rest_logic.go`: `rest` start-ongoing branch logic.
- `action_terminate_logic.go`: `terminate` finalize-only branch logic.
- `action_move_logic.go`: `move` action behavior and move path/position helpers.
- `action_retreat_logic.go`: `retreat` action behavior and retreat target/step helpers.
- `action_sleep_logic.go`: `sleep` action behavior and instant sleep settlement helper.
- `action_object_logic.go`: object-related cluster for `build`, `farm_plant`, `farm_harvest`, `container_deposit`, `container_withdraw`, plus shared object prepare/persist/state parsing helpers.
- `action_craft_eat_logic.go`: `craft` and `eat` action-specific precheck and execution.
- `action_settle_logic.go`: shared cross-action settle-and-plan path.
- `action_response_logic.go`: shared response shaping and world-time window helpers.
- `action_precheck_runtime.go`: shared framework runtime precheck (session/cooldown only).
- `action_handler_composition.go`: shared composition primitives for handler assembly.

## Non-Goals

- This document does not change runtime behavior by itself.
- This document does not prescribe package split yet; it is valid with single-file or multi-file organization inside `internal/app/action`.

## Minimal Pseudocode Shape

```go
func (e *ActionEngine) Execute(ctx context.Context, req Request) (Response, error) {
  return e.tx(ctx, func(txCtx context.Context) (Response, error) {
    ac, err := e.ValidateRequest(req)
    if err != nil { return Response{}, err }

    if out, ok, err := e.ReplayIdempotent(txCtx, &ac); err != nil || ok {
      return out, err
    }

    if err := e.LoadStateAndFinalizeOngoing(txCtx, &ac); err != nil {
      return Response{}, err
    }

    if err := e.ResolveSpec(&ac); err != nil { return Response{}, err }
    if err := e.BuildContext(txCtx, &ac); err != nil { return Response{}, err }
    if err := e.RunPrechecks(txCtx, &ac); err != nil { return Response{}, err }

    mode, err := e.ExecuteActionAndPlan(txCtx, &ac)
    if err != nil { return Response{}, err }

    if err := e.PersistAndRespond(txCtx, &ac); err != nil { return Response{}, err }
    if mode == ExecuteModeCompleted {
      return e.BuildCompletedResponse(&ac), nil
    }
    return e.BuildSettledResponse(&ac), nil
  })
}
```

## Adoption Notes

- Start by restructuring current use case into this pipeline without behavior changes.
- Keep tests green during each extraction step.
- Prefer incremental extraction: first stage methods, then optional file split.
- Preserve existing metrics semantics (`success`, `failure`, `conflict`) as a cross-cutting concern owned by engine.

## Refactor TODO

1. [x] Create task branch
- Command: `git checkout -b codex/refactor-action-engine-skeleton`
- Done criteria: dedicated branch created for this refactor only.

2. [x] Add guard tests before code movement
- Goal: lock current behavior before structural changes.
- Minimum coverage:
  - idempotent replay with same key;
  - `rest` early-complete still persists execution/state/event;
  - `terminate` only works for interruptible ongoing action;
  - persistence critical path semantics (state/execution/events/object/resource/session);
  - metrics semantics (`success`, `failure`, `conflict`).
- Done criteria: new/adjusted tests fail first (if behavior expectation is new), then pass with current logic.

3. [x] Introduce skeleton types (coexist with current flow)
- Add `ActionSpec` and `ActionContext` (`In/View/Plan/Tmp`) in new files under `internal/app/action`.
- Add pipeline method signatures for 8 steps; keep old `UseCase.Execute` behavior unchanged.
- Done criteria: code compiles and tests remain green.

4. [x] Extract stage methods from current `UseCase.Execute`
- Extract internal methods in order:
  - `ValidateRequest`
  - `ReplayIdempotent`
  - `LoadStateAndFinalizeOngoing`
  - `ResolveSpec`
  - `BuildContext`
  - `RunPrechecks`
  - `ExecuteActionAndPlan`
  - `PersistAndRespond`
- Done criteria: no behavior change; all existing and guard tests pass after each extraction.

5. [x] Switch `UseCase.Execute` to fixed pipeline skeleton
- Replace monolithic flow with stage orchestration only.
- Keep Engine-owned persistence invariant (single write owner).
- Done criteria: `UseCase.Execute` is short and stage-based; tests remain green.

6. [x] Introduce registry-based action specs incrementally
- Phase A: migrate `rest`, `sleep`, `terminate`, `gather`.
- Phase B: migrate `move`, `build`, `farm`, `container`, `craft`, `eat`, `retreat`.
- Done criteria: all supported action types are resolved from registry; no fallback to legacy branching.

7. [x] Enforce Plan-only persistence contract
- `PersistAndRespond` writes only from `ActionContext.Plan`.
- Handlers must not call repos directly.
- Add a lightweight `validatePlan` check before persistence.
- Done criteria: writes are centralized and deterministic by contract.

8. [x] Regression and integration checks
- Run package tests first: `go test ./internal/app/action/...`
- Prepare integration env when needed: `source scripts/setup_test_env.sh --prepare`
- Run targeted integration/e2e tests.
- Run full regression: `go test ./...`
- Done criteria: all test stages pass.

9. [x] Cleanup and documentation sync
- Remove obsolete branches/helpers from old monolithic flow.
- Keep docs aligned with final code:
  - `docs/engineering/action_usecase_engine_skeleton.md`
  - `docs/engineering/action_usecase_engine_skeleton_example.go.md`
- Done criteria: no stale flow description and no dead code paths.

10. [x] Suggested commit slicing
- Commit 1: guard tests only.
- Commit 2: skeleton types + pipeline signatures.
- Commit 3: execute flow migration to stage pipeline.
- Commit 4: action registry migration + cleanup.

## Handler Migration Plan

### Goal

- Move from "registry for routing only" to "registry + action-specific handlers for execution".
- Keep `Execute` pipeline fixed while removing action business branching from engine core.
- Preserve behavior parity (all current tests remain green).

### Done Criteria

1. Each supported action type is backed by a dedicated registered handler.
2. Engine action execution stage no longer contains broad action-type business branches.
3. Full regression passes (`go test ./...`).
4. This document and example document stay aligned with implementation.

### Phases

1. Phase A (core patterns first)
- Actions: `terminate`, `rest`, `sleep`, `gather`.
- Reason: covers early-complete + normal settle + resource side effects.
- Status: [x] completed

2. Phase B (movement and strategy)
- Actions: `move`, `retreat`.
- Reason: includes intent rewrite, pathing, threat-aware behavior.
- Status: [x] completed

3. Phase C (object state-machine actions)
- Actions: `build`, `farm_plant`, `farm_harvest`, `container_deposit`, `container_withdraw`.
- Reason: includes object precheck and object-state persistence paths.
- Status: [x] completed

4. Phase D (resource consumption actions)
- Actions: `craft`, `eat`.
- Reason: comparatively simple; finish migration with consistent handler model.
- Status: [x] completed

### Per-Phase Workflow (TDD)

1. Add or adjust guard tests for target actions.
2. Confirm test failure first when introducing new expectations.
3. Implement handlers and bind in registry.
4. Remove corresponding legacy branch from shared execution logic.
5. Run targeted package tests, then full package suite (`go test ./internal/app/action`).
6. At phase end, run full regression (`go test ./...`).

### Target Code Organization

- `engine_pipeline.go`: stage orchestration + shared persistence + cross-cutting concerns only.
- `registry.go` (or equivalent): action spec registration only.
- `handler_*.go`: per-action or tightly related action groups.
- Shared pure helpers stay centralized; repository writes stay in `PersistAndRespond`.

### Risk Controls

1. Early-complete regression risk
- Keep `rest/terminate` persistence + response semantics covered by tests.

2. Side-effect ordering risk
- Keep Plan-driven persistence order contract unchanged.

3. Logic duplication risk
- Extract reusable pure helpers; avoid duplicated repo-write logic across handlers.
