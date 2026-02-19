package action

import (
	"context"
	"fmt"

	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type retreatActionHandler struct{ BaseHandler }

func validateRetreatActionParams(survival.ActionIntent) bool {
	return true
}

func (h retreatActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h retreatActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func attachLastKnownThreat(result *survival.SettlementResult, snapshot world.Snapshot) {
	if result == nil {
		return
	}
	threat, ok := strongestVisibleThreat(snapshot)
	if !ok {
		return
	}
	for i := range result.Events {
		if result.Events[i].Type != "game_over" || result.Events[i].Payload == nil {
			continue
		}
		result.Events[i].Payload["last_known_threat"] = map[string]any{
			"id":           fmt.Sprintf("thr_%d_%d", threat.X, threat.Y),
			"type":         "wild",
			"pos":          map[string]int{"x": threat.X, "y": threat.Y},
			"danger_score": min(100, threat.BaseThreat*25),
		}
	}
}

func strongestVisibleThreat(snapshot world.Snapshot) (world.Tile, bool) {
	best := world.Tile{}
	found := false
	for _, tile := range snapshot.VisibleTiles {
		if tile.BaseThreat <= 0 {
			continue
		}
		if !found || tile.BaseThreat > best.BaseThreat {
			found = true
			best = tile
		}
	}
	return best, found
}

func resolveRetreatIntent(intent survival.ActionIntent, pos survival.Position, tiles []world.Tile) survival.ActionIntent {
	if intent.Type != survival.ActionRetreat {
		return intent
	}
	target, ok := highestThreatTile(pos, tiles)
	if !ok {
		return intent
	}
	dx, dy, ok := bestRetreatStep(pos, target, tiles)
	if !ok {
		return intent
	}
	intent.DX = dx
	intent.DY = dy
	return intent
}

func highestThreatTile(pos survival.Position, tiles []world.Tile) (world.Tile, bool) {
	best := world.Tile{}
	bestFound := false
	bestThreat := -1
	bestDist := 0
	for _, t := range tiles {
		if t.BaseThreat <= 0 {
			continue
		}
		dist := abs(t.X-pos.X) + abs(t.Y-pos.Y)
		if dist == 0 {
			continue
		}
		if !bestFound || t.BaseThreat > bestThreat || (t.BaseThreat == bestThreat && dist < bestDist) {
			best = t
			bestFound = true
			bestThreat = t.BaseThreat
			bestDist = dist
		}
	}
	return best, bestFound
}

func bestRetreatStep(pos survival.Position, threat world.Tile, tiles []world.Tile) (int, int, bool) {
	type dir struct {
		dx int
		dy int
	}
	candidates := []dir{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	visible := make(map[string]world.Tile, len(tiles))
	for _, t := range tiles {
		visible[posKey(t.X, t.Y)] = t
	}

	bestDX, bestDY := 0, 0
	bestFound := false
	bestDist := -1
	bestRisk := 9999
	for _, c := range candidates {
		tx := pos.X + c.dx
		ty := pos.Y + c.dy
		tile, ok := visible[posKey(tx, ty)]
		if !ok || !tile.Passable {
			continue
		}
		dist := abs(tx-threat.X) + abs(ty-threat.Y)
		risk := tile.BaseThreat
		if !bestFound || dist > bestDist || (dist == bestDist && risk < bestRisk) {
			bestFound = true
			bestDist = dist
			bestRisk = risk
			bestDX = c.dx
			bestDY = c.dy
		}
	}
	return bestDX, bestDY, bestFound
}

func posKey(x, y int) string {
	return fmt.Sprintf("%d:%d", x, y)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
