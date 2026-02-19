import { useMemo } from "react";
import type { ObserveResponse, ObserveTile } from "../../../types";
import { tileDetailCorner, tileKey } from "../model";

type UseMapViewModelResult = {
  tileMap: Map<string, ObserveTile>;
  resourceMap: Map<string, { type: string; is_depleted: boolean }>;
  objectMap: Map<string, Array<{ id: string; type: string }>>;
  xRange: number[];
  yRange: number[];
  effectiveSelectedTileId: string | null;
  selectedTile: ObserveTile | undefined;
  selectedResource: { type: string; is_depleted: boolean } | undefined;
  selectedObjects: Array<{ id: string; type: string }>;
  selectedCorner: string;
};

export function useMapViewModel(observe: ObserveResponse | undefined, selectedTileId: string | null): UseMapViewModelResult {
  const tileMap = useMemo(() => {
    const map = new Map<string, ObserveTile>();
    for (const tile of observe?.tiles ?? []) {
      map.set(tileKey(tile.pos), tile);
    }
    return map;
  }, [observe?.tiles]);
  const resourceMap = useMemo(() => {
    const map = new Map<string, { type: string; is_depleted: boolean }>();
    for (const resource of observe?.resources ?? []) {
      map.set(tileKey(resource.pos), { type: resource.type, is_depleted: resource.is_depleted });
    }
    return map;
  }, [observe?.resources]);
  const objectMap = useMemo(() => {
    const map = new Map<string, Array<{ id: string; type: string }>>();
    for (const obj of observe?.objects ?? []) {
      const key = tileKey(obj.pos);
      const prev = map.get(key) ?? [];
      prev.push({ id: obj.id, type: obj.type });
      map.set(key, prev);
    }
    return map;
  }, [observe?.objects]);
  const xRange = useMemo(() => {
    if (!observe) return [] as number[];
    const start = observe.view.center.x - observe.view.radius;
    const end = observe.view.center.x + observe.view.radius;
    return Array.from({ length: end - start + 1 }, (_, i) => start + i);
  }, [observe]);
  const yRange = useMemo(() => {
    if (!observe) return [] as number[];
    const start = observe.view.center.y - observe.view.radius;
    const end = observe.view.center.y + observe.view.radius;
    return Array.from({ length: end - start + 1 }, (_, i) => start + i);
  }, [observe]);
  const effectiveSelectedTileId =
    selectedTileId && tileMap.has(selectedTileId)
      ? selectedTileId
      : null;
  const selectedTile = effectiveSelectedTileId ? tileMap.get(effectiveSelectedTileId) : undefined;
  const selectedResource = selectedTile ? resourceMap.get(tileKey(selectedTile.pos)) : undefined;
  const selectedObjects = selectedTile ? (objectMap.get(tileKey(selectedTile.pos)) ?? []) : [];
  const selectedCorner = tileDetailCorner(selectedTile?.pos, observe?.view.center);

  return {
    tileMap,
    resourceMap,
    objectMap,
    xRange,
    yRange,
    effectiveSelectedTileId,
    selectedTile,
    selectedResource,
    selectedObjects,
    selectedCorner,
  };
}
