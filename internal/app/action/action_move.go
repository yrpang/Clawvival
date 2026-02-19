package action

import (
	"context"
	"fmt"

	"clawvival/internal/domain/survival"
	"clawvival/internal/domain/world"
)

type moveActionHandler struct{ BaseHandler }

func (h moveActionHandler) Precheck(ctx context.Context, uc UseCase, ac *ActionContext) error {
	return runStandardActionPrecheck(ctx, uc, ac)
}

func (h moveActionHandler) ExecuteActionAndPlan(ctx context.Context, uc UseCase, ac *ActionContext) (ExecuteMode, error) {
	return settleViaDomainOrInstant(ctx, uc, ac, settleOptions{applyObjectAction: true, createBuiltObjects: true})
}

func resolveMoveIntent(state survival.AgentStateAggregate, intent survival.ActionIntent, snapshot world.Snapshot) (survival.ActionIntent, error) {
	if intent.Type != survival.ActionMove {
		return intent, nil
	}
	if intent.Pos != nil {
		target := *intent.Pos
		if target.X == state.Position.X && target.Y == state.Position.Y {
			return intent, &ActionInvalidPositionError{TargetPos: &target}
		}
		return resolveMoveToPositionIntent(state.Position, intent, target, snapshot.VisibleTiles)
	}
	dx := intent.DX
	dy := intent.DY
	targetX := state.Position.X + dx
	targetY := state.Position.Y + dy
	targetPos := &survival.Position{X: targetX, Y: targetY}
	if abs(dx) > 1 || abs(dy) > 1 {
		return intent, &ActionInvalidPositionError{TargetPos: targetPos}
	}
	for _, tile := range snapshot.VisibleTiles {
		if tile.X == targetX && tile.Y == targetY {
			if tile.Passable {
				return intent, nil
			}
			return intent, &ActionInvalidPositionError{
				TargetPos:       targetPos,
				BlockingTilePos: &survival.Position{X: targetX, Y: targetY},
			}
		}
	}
	return intent, &ActionInvalidPositionError{TargetPos: targetPos}
}

func resolveMoveToPositionIntent(origin survival.Position, intent survival.ActionIntent, target survival.Position, visibleTiles []world.Tile) (survival.ActionIntent, error) {
	targetPos := &survival.Position{X: target.X, Y: target.Y}
	tileByPos := make(map[string]world.Tile, len(visibleTiles))
	for _, tile := range visibleTiles {
		tileByPos[movePosKey(tile.X, tile.Y)] = tile
	}
	targetTile, ok := tileByPos[movePosKey(target.X, target.Y)]
	if !ok {
		return intent, &ActionInvalidPositionError{TargetPos: targetPos}
	}
	if !targetTile.Passable {
		return intent, &ActionInvalidPositionError{
			TargetPos:       targetPos,
			BlockingTilePos: targetPos,
		}
	}
	if _, ok := tileByPos[movePosKey(origin.X, origin.Y)]; !ok {
		tileByPos[movePosKey(origin.X, origin.Y)] = world.Tile{X: origin.X, Y: origin.Y, Passable: true}
	}
	if !hasPassablePath(origin, target, tileByPos) {
		return intent, &ActionInvalidPositionError{TargetPos: targetPos}
	}
	intent.DX = target.X - origin.X
	intent.DY = target.Y - origin.Y
	return intent, nil
}

func hasPassablePath(origin survival.Position, target survival.Position, tileByPos map[string]world.Tile) bool {
	if origin.X == target.X && origin.Y == target.Y {
		return true
	}
	visited := map[string]bool{movePosKey(origin.X, origin.Y): true}
	queue := []survival.Position{{X: origin.X, Y: origin.Y}}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range dirs {
			nx := cur.X + d[0]
			ny := cur.Y + d[1]
			key := movePosKey(nx, ny)
			if visited[key] {
				continue
			}
			tile, ok := tileByPos[key]
			if !ok || !tile.Passable {
				continue
			}
			if nx == target.X && ny == target.Y {
				return true
			}
			visited[key] = true
			queue = append(queue, survival.Position{X: nx, Y: ny})
		}
	}
	return false
}

func movePosKey(x, y int) string {
	return fmt.Sprintf("%d,%d", x, y)
}
