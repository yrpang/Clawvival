import { useEffect, useMemo, useState } from "react";

type UseAgentUrlStateResult = {
  agentInput: string;
  setAgentInput: (value: string) => void;
  agentId: string;
  applyAgent: () => void;
};

export function useAgentUrlState(onApply: () => void): UseAgentUrlStateResult {
  const initialAgentId = useMemo(() => {
    const url = new URL(window.location.href);
    return url.searchParams.get("agent_id") ?? "";
  }, []);
  const [agentInput, setAgentInput] = useState(initialAgentId);
  const [agentId, setAgentId] = useState(initialAgentId);

  function applyAgent() {
    const next = agentInput.trim();
    if (!next) return;
    const url = new URL(window.location.href);
    url.searchParams.set("agent_id", next);
    window.history.replaceState(null, "", url.toString());
    setAgentId(next);
    onApply();
  }

  useEffect(() => {
    const url = new URL(window.location.href);
    if (agentId.trim()) {
      url.searchParams.set("agent_id", agentId.trim());
      window.history.replaceState(null, "", url.toString());
      return;
    }
    url.searchParams.delete("agent_id");
    window.history.replaceState(null, "", url.toString());
  }, [agentId]);

  useEffect(() => {
    function syncFromUrl() {
      const url = new URL(window.location.href);
      const next = url.searchParams.get("agent_id") ?? "";
      setAgentId(next);
      setAgentInput(next);
    }
    window.addEventListener("popstate", syncFromUrl);
    return () => window.removeEventListener("popstate", syncFromUrl);
  }, []);

  return {
    agentInput,
    setAgentInput,
    agentId,
    applyAgent,
  };
}
