//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestRemoteAPI_MainEndpoints(t *testing.T) {
	baseURL := strings.TrimRight(envOr("E2E_BASE_URL", "https://clawvival.app"), "/")
	client := &http.Client{Timeout: 20 * time.Second}
	envAgentID := strings.TrimSpace(os.Getenv("E2E_AGENT_ID"))
	envAgentKey := strings.TrimSpace(os.Getenv("E2E_AGENT_KEY"))

	requiredIntents := []string{
		"move",
		"gather",
		"craft",
		"build",
		"eat",
		"rest",
		"sleep",
		"farm_plant",
		"farm_harvest",
		"container_deposit",
		"container_withdraw",
		"retreat",
		"terminate",
	}

	t.Run("observe requires agent header", func(t *testing.T) {
		status, body := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/observe", "", "", map[string]any{})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("observe rejects invalid key", func(t *testing.T) {
		agentID, agentKey := mustRegisterAgent(t, client, baseURL)
		if strings.TrimSpace(envAgentID) != "" && strings.TrimSpace(envAgentKey) != "" {
			agentID, agentKey = envAgentID, envAgentKey
		}
		_ = agentKey
		status, body := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/observe", agentID, "invalid-key", map[string]any{})
		if status != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", status, string(body))
		}
	})

	t.Run("action rejects client dt field", func(t *testing.T) {
		agentID, agentKey := mustRegisterAgent(t, client, baseURL)
		status, body := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, map[string]any{
			"idempotency_key": "reject-dt-" + time.Now().UTC().Format("150405"),
			"intent": map[string]any{
				"type": "gather",
			},
			"dt": 30,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("skills endpoints", func(t *testing.T) {
		status, indexBody, err := doRequest(client, http.MethodGet, baseURL+"/skills/index.json", "", "", nil)
		if err != nil {
			t.Fatalf("skills index request: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("skills index status=%d body=%s", status, string(indexBody))
		}
		var index map[string]any
		if err := json.Unmarshal(indexBody, &index); err != nil {
			t.Fatalf("unmarshal skills index: %v body=%s", err, string(indexBody))
		}
		skills := asSlice(index["skills"])
		if len(skills) == 0 {
			t.Fatalf("expected skills array in index")
		}
		survival := map[string]any{}
		for _, item := range skills {
			m := asMap(item)
			if m["name"] == "survival" {
				survival = m
				break
			}
		}
		if len(survival) == 0 {
			t.Fatalf("expected survival skill in index, got=%v", index)
		}
		files := asSlice(survival["files"])
		expectedFiles := []string{
			"survival/skill.md",
			"survival/HEARTBEAT.md",
			"survival/MESSAGING.md",
			"survival/RULES.md",
			"survival/package.json",
		}
		for _, f := range expectedFiles {
			if !containsString(files, f) {
				t.Fatalf("expected file %q in skills index files=%v", f, files)
			}
		}

		status, fileBody, err := doRequest(client, http.MethodGet, baseURL+"/skills/survival/skill.md", "", "", nil)
		if err != nil {
			t.Fatalf("skills file request: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("skills file status=%d body=%s", status, string(fileBody))
		}
		if len(fileBody) == 0 {
			t.Fatalf("skills file empty")
		}

		for _, p := range []string{
			"/skills/survival/HEARTBEAT.md",
			"/skills/survival/MESSAGING.md",
			"/skills/survival/RULES.md",
			"/skills/survival/package.json",
		} {
			status, body, err := doRequest(client, http.MethodGet, baseURL+p, "", "", nil)
			if err != nil {
				t.Fatalf("skills file request %s: %v", p, err)
			}
			if status != http.StatusOK {
				t.Fatalf("skills file %s status=%d body=%s", p, status, string(body))
			}
			if len(body) == 0 {
				t.Fatalf("skills file %s is empty", p)
			}
		}
	})

	t.Run("observe and status contract coverage", func(t *testing.T) {
		agentID, agentKey := mustRegisterAgent(t, client, baseURL)
		status, observeBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/observe", agentID, agentKey, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("observe status=%d body=%s", status, string(observeBody))
		}
		var observe map[string]any
		if err := json.Unmarshal(observeBody, &observe); err != nil {
			t.Fatalf("unmarshal observe response: %v body=%s", err, string(observeBody))
		}

		view := asMap(observe["view"])
		if toNum(view["width"]) != 11 || toNum(view["height"]) != 11 || toNum(view["radius"]) != 5 {
			t.Fatalf("unexpected observe view contract: %v", view)
		}
		tiles := asSlice(observe["tiles"])
		if got, want := len(tiles), 121; got != want {
			t.Fatalf("expected %d tiles in fixed 11x11 view, got=%d", want, got)
		}
		agentState := asMap(observe["agent_state"])
		sessionID, _ := agentState["session_id"].(string)
		if strings.TrimSpace(sessionID) == "" {
			t.Fatalf("expected session_id in observe.agent_state, got=%v", agentState)
		}
		assertActionCostsContainIntents(t, asMap(observe["action_costs"]), requiredIntents)
		assertRulesContract(t, asMap(asMap(observe["world"])["rules"]))
		assertEntitiesOnVisibleTiles(t, observe)

		status, statusBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/status", agentID, agentKey, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("status endpoint status=%d body=%s", status, string(statusBody))
		}
		var st map[string]any
		if err := json.Unmarshal(statusBody, &st); err != nil {
			t.Fatalf("unmarshal status response: %v body=%s", err, string(statusBody))
		}
		statusState := asMap(st["agent_state"])
		if got, _ := statusState["session_id"].(string); strings.TrimSpace(got) == "" || got != sessionID {
			t.Fatalf("expected same session_id from observe/status, observe=%q status=%q", sessionID, got)
		}
		timeOfDay, _ := st["time_of_day"].(string)
		if strings.TrimSpace(timeOfDay) == "" {
			t.Fatalf("expected time_of_day in status response, got=%v", st)
		}
		assertActionCostsContainIntents(t, asMap(st["action_costs"]), requiredIntents)
		assertRulesContract(t, asMap(asMap(st["world"])["rules"]))
	})

	t.Run("action intent set and rejection contract", func(t *testing.T) {
		agentID, agentKey := mustRegisterAgent(t, client, baseURL)
		status, observeBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/observe", agentID, agentKey, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("observe before intent contract status=%d body=%s", status, string(observeBody))
		}
		var observe map[string]any
		if err := json.Unmarshal(observeBody, &observe); err != nil {
			t.Fatalf("unmarshal observe before intent contract: %v body=%s", err, string(observeBody))
		}
		movePos := pickAdjacentMovePos(observe)
		if movePos == nil {
			t.Fatalf("expected at least one adjacent visible walkable pos for move.pos contract")
		}

		cases := []struct {
			name       string
			intent     map[string]any
			wantStatus int
			wantCode   string
		}{
			{name: "move with pos", intent: map[string]any{"type": "move", "pos": movePos}, wantStatus: http.StatusOK},
			{name: "move missing direction", intent: map[string]any{"type": "move"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "gather missing target", intent: map[string]any{"type": "gather"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "craft missing recipe", intent: map[string]any{"type": "craft"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "build missing pos", intent: map[string]any{"type": "build", "object_type": "bed_rough"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "eat missing count", intent: map[string]any{"type": "eat", "item_type": "berry"}, wantStatus: http.StatusConflict, wantCode: "action_precondition_failed"},
			{name: "rest out of range", intent: map[string]any{"type": "rest", "rest_minutes": 121}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "sleep missing bed id", intent: map[string]any{"type": "sleep"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "farm plant missing farm id", intent: map[string]any{"type": "farm_plant"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "farm harvest missing farm id", intent: map[string]any{"type": "farm_harvest"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "container deposit missing items", intent: map[string]any{"type": "container_deposit", "container_id": "obj-x"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "container withdraw missing items", intent: map[string]any{"type": "container_withdraw", "container_id": "obj-x"}, wantStatus: http.StatusBadRequest, wantCode: "invalid_action_params"},
			{name: "retreat no params", intent: map[string]any{"type": "retreat"}, wantStatus: http.StatusOK},
			{name: "terminate no ongoing", intent: map[string]any{"type": "terminate"}, wantStatus: http.StatusConflict, wantCode: "action_precondition_failed"},
			{name: "unsupported combat intent", intent: map[string]any{"type": "combat"}, wantStatus: http.StatusBadRequest, wantCode: "bad_request"},
			{name: "gather out of view", intent: map[string]any{"type": "gather", "target_id": "res_999_999_wood"}, wantStatus: http.StatusConflict, wantCode: "TARGET_OUT_OF_VIEW"},
		}

		for i, tc := range cases {
			key := "intent-contract-" + tc.name + "-" + time.Now().UTC().Format("150405") + "-" + strings.ReplaceAll(strings.ToLower(tc.name), " ", "-")
			status, body := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, map[string]any{
				"idempotency_key": key + "-" + string(rune('a'+i)),
				"intent":          tc.intent,
			})
			if status != tc.wantStatus {
				t.Fatalf("%s: expected status=%d got=%d body=%s", tc.name, tc.wantStatus, status, string(body))
			}
			var resp map[string]any
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("%s: unmarshal response: %v body=%s", tc.name, err, string(body))
			}
			if status == http.StatusOK {
				if code, _ := resp["result_code"].(string); code != "OK" {
					t.Fatalf("%s: expected result_code=OK, got=%v body=%s", tc.name, resp["result_code"], string(body))
				}
				continue
			}
			assertRejectedContract(t, tc.name, resp, tc.wantCode)
		}
	})

	t.Run("observe action status replay ops", func(t *testing.T) {
		agentID, agentKey := mustRegisterAgent(t, client, baseURL)
		idempotencyKey := "remote-e2e-" + time.Now().UTC().Format("20060102150405")
		status, observeBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/observe", agentID, agentKey, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("observe status=%d body=%s", status, string(observeBody))
		}
		var observe map[string]any
		if err := json.Unmarshal(observeBody, &observe); err != nil {
			t.Fatalf("unmarshal observe response: %v body=%s", err, string(observeBody))
		}
		moveDirection := pickMoveDirection(observe)
		if strings.TrimSpace(moveDirection) == "" {
			t.Fatalf("expected at least one walkable adjacent tile in observe response")
		}

		actionReq := map[string]any{
			"idempotency_key": idempotencyKey,
			"intent": map[string]any{
				"type":      "move",
				"direction": moveDirection,
			},
			"strategy_hash": "remote-e2e",
		}
		status, firstActionBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, actionReq)
		if status != http.StatusOK {
			t.Fatalf("first action status=%d body=%s", status, string(firstActionBody))
		}
		var first map[string]any
		if err := json.Unmarshal(firstActionBody, &first); err != nil {
			t.Fatalf("unmarshal first action: %v body=%s", err, string(firstActionBody))
		}

		status, secondActionBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, actionReq)
		if status != http.StatusOK {
			t.Fatalf("second action status=%d body=%s", status, string(secondActionBody))
		}
		var second map[string]any
		if err := json.Unmarshal(secondActionBody, &second); err != nil {
			t.Fatalf("unmarshal second action: %v body=%s", err, string(secondActionBody))
		}
		if asMap(first["updated_state"])["version"] != asMap(second["updated_state"])["version"] {
			t.Fatalf("idempotency mismatch: first=%v second=%v", first["updated_state"], second["updated_state"])
		}
		events := asSlice(first["events"])
		if len(events) == 0 {
			t.Fatalf("expected action_settled event in successful action response")
		}
		actionSettled := asMap(events[0])
		payload := asMap(actionSettled["payload"])
		if got, _ := payload["agent_id"].(string); got != agentID {
			t.Fatalf("expected event payload agent_id=%q, got=%q payload=%v", agentID, got, payload)
		}
		if got, _ := payload["session_id"].(string); strings.TrimSpace(got) == "" {
			t.Fatalf("expected event payload session_id, got payload=%v", payload)
		}
		decision := asMap(payload["decision"])
		if intent, _ := decision["intent"].(string); intent != "move" {
			t.Fatalf("expected decision.intent=move, got=%v payload=%v", decision["intent"], payload)
		}
		if _, ok := decision["params"]; !ok {
			t.Fatalf("expected decision.params in action_settled payload=%v", payload)
		}
		if _, ok := decision["dt_minutes"]; !ok {
			t.Fatalf("expected decision.dt_minutes in action_settled payload=%v", payload)
		}
		result := asMap(payload["result"])
		if _, ok := result["hp_loss"]; !ok {
			t.Fatalf("expected result.hp_loss in action_settled payload=%v", payload)
		}

		status, statusBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/status", agentID, agentKey, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("status endpoint status=%d body=%s", status, string(statusBody))
		}
		var st map[string]any
		if err := json.Unmarshal(statusBody, &st); err != nil {
			t.Fatalf("unmarshal status response: %v body=%s", err, string(statusBody))
		}
		timeOfDay, _ := st["time_of_day"].(string)
		if strings.TrimSpace(timeOfDay) == "" {
			t.Fatalf("expected time_of_day in status response, got=%v", st)
		}
		sessionID, _ := asMap(st["agent_state"])["session_id"].(string)
		if strings.TrimSpace(sessionID) == "" {
			t.Fatalf("expected session_id in status response, got=%v", st)
		}

		replayURL := baseURL + "/api/agent/replay?limit=20&session_id=" + sessionID
		status, replayBody, err := doRequest(client, http.MethodGet, replayURL, agentID, agentKey, nil)
		if err != nil {
			t.Fatalf("replay request: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("replay status=%d body=%s", status, string(replayBody))
		}
		var rep map[string]any
		if err := json.Unmarshal(replayBody, &rep); err != nil {
			t.Fatalf("unmarshal replay response: %v body=%s", err, string(replayBody))
		}
		if len(asSlice(rep["events"])) == 0 {
			t.Fatalf("expected replay events in response")
		}
		status, emptyReplayBody, err := doRequest(client, http.MethodGet, baseURL+"/api/agent/replay?limit=20&session_id=session-non-existent", agentID, agentKey, nil)
		if err != nil {
			t.Fatalf("filtered replay request: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("filtered replay status=%d body=%s", status, string(emptyReplayBody))
		}
		var emptyReplay map[string]any
		if err := json.Unmarshal(emptyReplayBody, &emptyReplay); err != nil {
			t.Fatalf("unmarshal filtered replay response: %v body=%s", err, string(emptyReplayBody))
		}
		if got := len(asSlice(emptyReplay["events"])); got != 0 {
			t.Fatalf("expected empty replay for unmatched session, got=%d body=%s", got, string(emptyReplayBody))
		}

		status, kpiBody, err := doRequest(client, http.MethodGet, baseURL+"/ops/kpi", "", "", nil)
		if err != nil {
			t.Fatalf("kpi request: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("kpi status=%d body=%s", status, string(kpiBody))
		}
		var kpi map[string]any
		if err := json.Unmarshal(kpiBody, &kpi); err != nil {
			t.Fatalf("unmarshal kpi: %v body=%s", err, string(kpiBody))
		}
		if _, ok := kpi["action_total"]; !ok {
			t.Fatalf("expected action_total in kpi response")
		}
	})

	t.Run("rest can be terminated and settles proportionally", func(t *testing.T) {
		agentID, agentKey := mustRegisterAgent(t, client, baseURL)
		restStartKey := "remote-rest-start-" + time.Now().UTC().Format("150405")
		terminateKey := "remote-rest-term-" + time.Now().UTC().Format("150405")
		postTerminateActionKey := "remote-after-term-" + time.Now().UTC().Format("150405")
		status, observeBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/observe", agentID, agentKey, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("observe before rest status=%d body=%s", status, string(observeBody))
		}
		var observe map[string]any
		if err := json.Unmarshal(observeBody, &observe); err != nil {
			t.Fatalf("unmarshal observe before rest: %v body=%s", err, string(observeBody))
		}
		moveDirection := pickMoveDirection(observe)
		if strings.TrimSpace(moveDirection) == "" {
			t.Fatalf("expected at least one walkable adjacent tile in observe response")
		}

		status, restBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, map[string]any{
			"idempotency_key": restStartKey,
			"intent": map[string]any{
				"type":         "rest",
				"rest_minutes": 2,
			},
		})
		if status != http.StatusOK {
			t.Fatalf("rest start status=%d body=%s", status, string(restBody))
		}
		var restResp map[string]any
		if err := json.Unmarshal(restBody, &restResp); err != nil {
			t.Fatalf("unmarshal rest start: %v body=%s", err, string(restBody))
		}
		ongoing := asMap(asMap(restResp["updated_state"])["ongoing_action"])
		if ongoing["type"] != "rest" {
			t.Fatalf("expected ongoing rest action, got=%v", ongoing)
		}

		status, blockedBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, map[string]any{
			"idempotency_key": "remote-blocked-" + time.Now().UTC().Format("150405"),
			"intent": map[string]any{
				"type":      "move",
				"direction": moveDirection,
			},
		})
		if status != http.StatusConflict {
			t.Fatalf("expected 409 while resting, got %d body=%s", status, string(blockedBody))
		}

		time.Sleep(2 * time.Second)

		status, terminateBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, map[string]any{
			"idempotency_key": terminateKey,
			"intent": map[string]any{
				"type": "terminate",
			},
		})
		if status != http.StatusOK {
			t.Fatalf("terminate status=%d body=%s", status, string(terminateBody))
		}
		var termResp map[string]any
		if err := json.Unmarshal(terminateBody, &termResp); err != nil {
			t.Fatalf("unmarshal terminate: %v body=%s", err, string(terminateBody))
		}
		if asMap(termResp["updated_state"])["ongoing_action"] != nil {
			t.Fatalf("expected ongoing action cleared after terminate")
		}

		foundEnded := false
		for _, evtAny := range asSlice(termResp["events"]) {
			evt := asMap(evtAny)
			if evt["type"] != "ongoing_action_ended" {
				continue
			}
			payload := asMap(evt["payload"])
			if payload["forced"] != true {
				t.Fatalf("expected forced=true in ongoing_action_ended, got=%v", payload)
			}
			actualMinutes, _ := payload["actual_minutes"].(float64)
			plannedMinutes, _ := payload["planned_minutes"].(float64)
			if actualMinutes < 0 {
				t.Fatalf("expected actual_minutes>=0, got=%v payload=%v", actualMinutes, payload)
			}
			if plannedMinutes != 2 {
				t.Fatalf("expected planned_minutes=2, got=%v payload=%v", plannedMinutes, payload)
			}
			if actualMinutes >= plannedMinutes {
				t.Fatalf("expected early terminate actual_minutes < planned_minutes, got actual=%v planned=%v payload=%v", actualMinutes, plannedMinutes, payload)
			}
			foundEnded = true
			break
		}
		if !foundEnded {
			t.Fatalf("expected ongoing_action_ended event in terminate response")
		}

		status, afterBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, agentKey, map[string]any{
			"idempotency_key": postTerminateActionKey,
			"intent": map[string]any{
				"type":         "rest",
				"rest_minutes": 1,
			},
		})
		if status != http.StatusOK {
			t.Fatalf("expected action available after terminate, got=%d body=%s", status, string(afterBody))
		}
	})
}

func mustRegisterAgent(t *testing.T, client *http.Client, baseURL string) (string, string) {
	t.Helper()
	status, regBody := mustJSONWithAuth(t, client, http.MethodPost, baseURL+"/api/agent/register", "", "", map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", status, string(regBody))
	}
	var reg map[string]any
	if err := json.Unmarshal(regBody, &reg); err != nil {
		t.Fatalf("unmarshal register response: %v body=%s", err, string(regBody))
	}
	agentID, _ := reg["agent_id"].(string)
	agentKey, _ := reg["agent_key"].(string)
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(agentKey) == "" {
		t.Fatalf("register returned empty credentials: %v", reg)
	}
	return agentID, agentKey
}

func assertActionCostsContainIntents(t *testing.T, costs map[string]any, intents []string) {
	t.Helper()
	for _, intent := range intents {
		if _, ok := costs[intent]; !ok {
			t.Fatalf("expected action_costs to contain intent=%q, got=%v", intent, costs)
		}
	}
}

func assertRulesContract(t *testing.T, rules map[string]any) {
	t.Helper()
	if toNum(rules["standard_tick_minutes"]) != 30 {
		t.Fatalf("expected rules.standard_tick_minutes=30, got=%v", rules["standard_tick_minutes"])
	}
	if len(asMap(rules["drains_per_30m"])) == 0 {
		t.Fatalf("expected rules.drains_per_30m")
	}
	if len(asMap(rules["thresholds"])) == 0 {
		t.Fatalf("expected rules.thresholds")
	}
	if len(asMap(rules["visibility"])) == 0 {
		t.Fatalf("expected rules.visibility")
	}
	if len(asMap(rules["farming"])) == 0 {
		t.Fatalf("expected rules.farming")
	}
	if len(asMap(rules["seed"])) == 0 {
		t.Fatalf("expected rules.seed")
	}
}

func assertEntitiesOnVisibleTiles(t *testing.T, observe map[string]any) {
	t.Helper()
	visibleByPos := map[string]bool{}
	for _, tileAny := range asSlice(observe["tiles"]) {
		tile := asMap(tileAny)
		if visible, _ := tile["is_visible"].(bool); !visible {
			continue
		}
		pos := asMap(tile["pos"])
		x, xOK := asInt(pos["x"])
		y, yOK := asInt(pos["y"])
		if xOK && yOK {
			visibleByPos[posKey(x, y)] = true
		}
	}

	for _, resourceAny := range asSlice(observe["resources"]) {
		resource := asMap(resourceAny)
		id, _ := resource["id"].(string)
		if !strings.HasPrefix(id, "res_") {
			t.Fatalf("expected resource id prefix res_, got=%q", id)
		}
		assertEntityVisible(t, "resource", resource, visibleByPos)
	}
	for _, objectAny := range asSlice(observe["objects"]) {
		assertEntityVisible(t, "object", asMap(objectAny), visibleByPos)
	}
	for _, threatAny := range asSlice(observe["threats"]) {
		threat := asMap(threatAny)
		id, _ := threat["id"].(string)
		if !strings.HasPrefix(id, "thr_") {
			t.Fatalf("expected threat id prefix thr_, got=%q", id)
		}
		assertEntityVisible(t, "threat", threat, visibleByPos)
	}
}

func assertEntityVisible(t *testing.T, kind string, entity map[string]any, visibleByPos map[string]bool) {
	t.Helper()
	pos := asMap(entity["pos"])
	x, xOK := asInt(pos["x"])
	y, yOK := asInt(pos["y"])
	if !xOK || !yOK {
		t.Fatalf("expected %s pos with x/y, got=%v", kind, entity)
	}
	if !visibleByPos[posKey(x, y)] {
		t.Fatalf("expected %s on visible tile, entity=%v", kind, entity)
	}
}

func assertRejectedContract(t *testing.T, caseName string, resp map[string]any, wantCode string) {
	t.Helper()
	if got, _ := resp["result_code"].(string); got != "REJECTED" {
		t.Fatalf("%s: expected result_code=REJECTED, got=%v resp=%v", caseName, resp["result_code"], resp)
	}
	errObj := asMap(resp["error"])
	if code, _ := errObj["code"].(string); code != wantCode {
		t.Fatalf("%s: expected error.code=%q got=%q resp=%v", caseName, wantCode, code, resp)
	}
	if _, ok := errObj["retryable"]; !ok {
		t.Fatalf("%s: expected error.retryable field, resp=%v", caseName, resp)
	}
	if _, ok := errObj["blocked_by"]; !ok {
		t.Fatalf("%s: expected error.blocked_by field, resp=%v", caseName, resp)
	}
	if _, ok := errObj["details"]; !ok {
		t.Fatalf("%s: expected error.details field, resp=%v", caseName, resp)
	}
	actionErr := asMap(resp["action_error"])
	if code, _ := actionErr["code"].(string); code != wantCode {
		t.Fatalf("%s: expected action_error.code=%q got=%q resp=%v", caseName, wantCode, code, resp)
	}
}

func mustJSONWithAuth(t *testing.T, client *http.Client, method, url, agentID, agentKey string, body map[string]any) (int, []byte) {
	t.Helper()
	status, respBody, err := doRequest(client, method, url, agentID, agentKey, body)
	if err != nil {
		t.Fatalf("%s %s request failed: %v", method, url, err)
	}
	return status, respBody
}

func doRequest(client *http.Client, method, url, agentID, agentKey string, body map[string]any) (int, []byte, error) {
	var payloadBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		payloadBytes = b
	}

	var lastStatus int
	var lastBody []byte
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		var payload io.Reader
		if len(payloadBytes) > 0 {
			payload = bytes.NewReader(payloadBytes)
		}
		req, err := http.NewRequest(method, url, payload)
		if err != nil {
			return 0, nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if strings.TrimSpace(agentID) != "" {
			req.Header.Set("X-Agent-ID", agentID)
		}
		if strings.TrimSpace(agentKey) != "" {
			req.Header.Set("X-Agent-Key", agentKey)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		lastStatus, lastBody, lastErr = resp.StatusCode, respBody, nil
		if resp.StatusCode >= 500 {
			time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			continue
		}
		return resp.StatusCode, respBody, nil
	}
	if lastErr != nil {
		return 0, nil, lastErr
	}
	return lastStatus, lastBody, nil
}

func envOr(k, def string) string {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	return v
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func containsString(in []any, want string) bool {
	for _, v := range in {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

func posKey(x, y int) string {
	return strings.Join([]string{toString(x), toString(y)}, ",")
}

func toString(v int) string {
	return strconv.Itoa(v)
}

func pickMoveDirection(observe map[string]any) string {
	center := asMap(asMap(observe["view"])["center"])
	cx, cxOK := asInt(center["x"])
	cy, cyOK := asInt(center["y"])
	if !cxOK || !cyOK {
		return ""
	}
	for _, tile := range asSlice(observe["tiles"]) {
		tileMap := asMap(tile)
		if walkable, _ := tileMap["is_walkable"].(bool); !walkable {
			continue
		}
		if visible, _ := tileMap["is_visible"].(bool); !visible {
			continue
		}
		pos := asMap(tileMap["pos"])
		x, xOk := asInt(pos["x"])
		y, yOk := asInt(pos["y"])
		if !xOk || !yOk {
			continue
		}
		if x == cx+1 && y == cy {
			return "E"
		}
		if x == cx-1 && y == cy {
			return "W"
		}
		if x == cx && y == cy+1 {
			return "N"
		}
		if x == cx && y == cy-1 {
			return "S"
		}
	}
	return ""
}

func pickAdjacentMovePos(observe map[string]any) map[string]any {
	center := asMap(asMap(observe["view"])["center"])
	cx, cxOK := asInt(center["x"])
	cy, cyOK := asInt(center["y"])
	if !cxOK || !cyOK {
		return nil
	}
	for _, tile := range asSlice(observe["tiles"]) {
		tileMap := asMap(tile)
		if walkable, _ := tileMap["is_walkable"].(bool); !walkable {
			continue
		}
		if visible, _ := tileMap["is_visible"].(bool); !visible {
			continue
		}
		pos := asMap(tileMap["pos"])
		x, xOk := asInt(pos["x"])
		y, yOk := asInt(pos["y"])
		if !xOk || !yOk {
			continue
		}
		if absInt(x-cx)+absInt(y-cy) != 1 {
			continue
		}
		return map[string]any{"x": x, "y": y}
	}
	return nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func toNum(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
