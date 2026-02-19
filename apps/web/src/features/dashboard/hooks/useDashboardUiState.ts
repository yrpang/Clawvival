import { useState } from "react";

type UseDashboardUiStateResult = {
  page: number;
  setPage: (updater: number | ((value: number) => number)) => void;
  expandedId: string | null;
  setExpandedId: (updater: string | null | ((value: string | null) => string | null)) => void;
  actionFilter: string;
  fromTime: string;
  toTime: string;
  selectedTileId: string | null;
  setSelectedTileId: (updater: string | null | ((value: string | null) => string | null)) => void;
  resetForAgentChange: () => void;
  setActionFilterAndResetPage: (value: string) => void;
  setFromTimeAndResetPage: (value: string) => void;
  setToTimeAndResetPage: (value: string) => void;
  clearExpanded: () => void;
};

export function useDashboardUiState(): UseDashboardUiStateResult {
  const [page, setPage] = useState(1);
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const [actionFilter, setActionFilter] = useState("");
  const [fromTime, setFromTime] = useState("");
  const [toTime, setToTime] = useState("");
  const [selectedTileId, setSelectedTileId] = useState<string | null>(null);

  function resetForAgentChange() {
    setPage(1);
    setExpandedId(null);
    setSelectedTileId(null);
    setActionFilter("");
    setFromTime("");
    setToTime("");
  }

  function setActionFilterAndResetPage(value: string) {
    setActionFilter(value);
    setPage(1);
    setExpandedId(null);
  }

  function setFromTimeAndResetPage(value: string) {
    setFromTime(value);
    setPage(1);
    setExpandedId(null);
  }

  function setToTimeAndResetPage(value: string) {
    setToTime(value);
    setPage(1);
    setExpandedId(null);
  }

  function clearExpanded() {
    setExpandedId(null);
  }

  return {
    page,
    setPage,
    expandedId,
    setExpandedId,
    actionFilter,
    fromTime,
    toTime,
    selectedTileId,
    setSelectedTileId,
    resetForAgentChange,
    setActionFilterAndResetPage,
    setFromTimeAndResetPage,
    setToTimeAndResetPage,
    clearExpanded,
  };
}
