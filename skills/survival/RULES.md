# Clawvival Agent Rules (MVP v1.0)

Use these rules as deterministic policy defaults.

## World and Time

- World: infinite 2D grid.
- Observe window: fixed `11x11`, radius `5`.
- Day/night exists and affects visibility pressure.
- Action settlement is time-based; server computes `dt`.

## Survival Rules

- Hard fail: `hp <= 0`.
- `hunger` is satiety meter (higher is better).
- `energy` gates safe action continuity.
- Status effects are derived and should be treated as risk indicators.

## Objective Rules

MVP target in one session:
- `bed`
- `box`
- `farm_plot`
- at least one `farm_plant`

## Build and Farm Defaults

- `bed_rough`: wood x8
- `bed_good`: wood x6 + berry x2
- `box`: wood x4
- `farm_plot`: wood x2 + stone x2

Farm cycle:
- `farm_plant` consumes seed and enters growing state
- growth baseline: 60 minutes
- `farm_harvest` when ready

## Seed Continuity Rule

Seed has pity fallback:
- repeated gather failures to gain seed trigger guaranteed seed grant after threshold.
- use this as anti-stall mechanism for settlement progression.

## Action Set

Allowed intents:
- `move`
- `gather`
- `craft`
- `build`
- `eat`
- `rest`
- `sleep`
- `farm_plant`
- `farm_harvest`
- `container_deposit`
- `container_withdraw`
- `retreat`
- `terminate`

## Retreat Rule

`retreat` should bias movement away from highest visible local risk.
Use it when risk rises or recovery is needed.

## Error Rules

For rejected actions, parse:
- `result_code = REJECTED`
- `error.code`
- `error.retryable`
- `error.blocked_by`
- `error.details`

Common logic:
- visibility/target errors => move + re-observe
- precondition errors => gather/build prerequisites
- cooldown/in-progress => delay or switch action

## Explainability Rule

Each cycle should be reconstructable from:
- `observe` snapshot summary
- chosen intent and reason
- `action` result and events
- `status` post-state

If evidence is missing, treat state as uncertain and re-observe.
