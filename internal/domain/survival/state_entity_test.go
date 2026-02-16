package survival

import "testing"

func TestAgentStateInventoryOps(t *testing.T) {
	s := AgentStateAggregate{}
	s.AddItem("wood", 3)
	s.AddItem("wood", 2)
	if s.Inventory["wood"] != 5 {
		t.Fatalf("expected wood=5, got %d", s.Inventory["wood"])
	}
	if ok := s.ConsumeItem("wood", 4); !ok {
		t.Fatalf("expected consume success")
	}
	if s.Inventory["wood"] != 1 {
		t.Fatalf("expected wood=1, got %d", s.Inventory["wood"])
	}
	if ok := s.ConsumeItem("wood", 2); ok {
		t.Fatalf("expected consume failure when insufficient")
	}
}

func TestAgentStateMarkDeathCause(t *testing.T) {
	s := AgentStateAggregate{}
	s.MarkDead(DeathCauseCombat)
	if !s.Dead {
		t.Fatalf("expected dead=true")
	}
	if s.DeathCause != DeathCauseCombat {
		t.Fatalf("unexpected death cause: %s", s.DeathCause)
	}
}

func TestAgentSessionOpenAndClose(t *testing.T) {
	session := NewSession("agent-1", 1)
	if session.Status != SessionAlive {
		t.Fatalf("expected alive session")
	}
	session.Close(DeathCauseStarvation)
	if session.Status != SessionDead {
		t.Fatalf("expected dead session")
	}
	if session.DeathCause != DeathCauseStarvation {
		t.Fatalf("unexpected death cause: %s", session.DeathCause)
	}
}
