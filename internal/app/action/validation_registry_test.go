package action

import (
	"testing"

	"clawvival/internal/domain/survival"
)

func TestSupportedActionTypes_MatchesRegistry(t *testing.T) {
	registry := actionRegistry()
	supported := supportedActionTypes()
	validators := actionParamValidators()

	for _, actionType := range supported {
		if _, ok := registry[actionType]; !ok {
			t.Fatalf("supported action %q missing from registry", actionType)
		}
		if !isSupportedActionType(actionType) {
			t.Fatalf("supported action %q must be accepted", actionType)
		}
		if _, ok := validators[actionType]; !ok {
			t.Fatalf("supported action %q missing param validator", actionType)
		}
	}

	for actionType := range registry {
		if !containsActionType(supported, actionType) {
			t.Fatalf("registry action %q missing from supported list", actionType)
		}
	}

	if isSupportedActionType(survival.ActionType("__invalid__")) {
		t.Fatal("invalid action type must not be accepted")
	}
}

func containsActionType(types []survival.ActionType, target survival.ActionType) bool {
	for _, actionType := range types {
		if actionType == target {
			return true
		}
	}
	return false
}
