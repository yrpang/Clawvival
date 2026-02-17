//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRemoteAPI_MainEndpoints(t *testing.T) {
	baseURL := strings.TrimRight(envOr("E2E_BASE_URL", "https://clawverse.fly.dev"), "/")
	agentID := envOr("E2E_AGENT_ID", "demo-agent")
	client := &http.Client{Timeout: 20 * time.Second}

	t.Run("observe requires agent header", func(t *testing.T) {
		status, body := mustJSON(t, client, http.MethodPost, baseURL+"/api/agent/observe", "", map[string]any{})
		if status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", status, string(body))
		}
	})

	t.Run("skills endpoints", func(t *testing.T) {
		status, indexBody, err := doRequest(client, http.MethodGet, baseURL+"/skills/index.json", "", nil)
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

		status, fileBody, err := doRequest(client, http.MethodGet, baseURL+"/skills/survival/skill.md", "", nil)
		if err != nil {
			t.Fatalf("skills file request: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("skills file status=%d body=%s", status, string(fileBody))
		}
		if len(fileBody) == 0 {
			t.Fatalf("skills file empty")
		}
	})

	idempotencyKey := "remote-e2e-" + time.Now().UTC().Format("20060102150405")

	t.Run("observe action status replay ops", func(t *testing.T) {
		status, observeBody := mustJSON(t, client, http.MethodPost, baseURL+"/api/agent/observe", agentID, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("observe status=%d body=%s", status, string(observeBody))
		}

		actionReq := map[string]any{
			"idempotency_key": idempotencyKey,
			"intent": map[string]any{
				"type": "gather",
			},
			"dt":            30,
			"strategy_hash": "remote-e2e",
		}
		status, firstActionBody := mustJSON(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, actionReq)
		if status != http.StatusOK {
			t.Fatalf("first action status=%d body=%s", status, string(firstActionBody))
		}
		var first map[string]any
		if err := json.Unmarshal(firstActionBody, &first); err != nil {
			t.Fatalf("unmarshal first action: %v body=%s", err, string(firstActionBody))
		}

		status, secondActionBody := mustJSON(t, client, http.MethodPost, baseURL+"/api/agent/action", agentID, actionReq)
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

		status, statusBody := mustJSON(t, client, http.MethodPost, baseURL+"/api/agent/status", agentID, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("status endpoint status=%d body=%s", status, string(statusBody))
		}
		var st map[string]any
		if err := json.Unmarshal(statusBody, &st); err != nil {
			t.Fatalf("unmarshal status response: %v body=%s", err, string(statusBody))
		}
		timeOfDay, _ := st["TimeOfDay"].(string)
		if strings.TrimSpace(timeOfDay) == "" {
			t.Fatalf("expected TimeOfDay in status response, got=%v", st)
		}

		replayURL := baseURL + "/api/agent/replay?limit=20"
		status, replayBody, err := doRequest(client, http.MethodGet, replayURL, agentID, nil)
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
		if len(asSlice(rep["Events"])) == 0 {
			t.Fatalf("expected replay events in response")
		}

		status, kpiBody, err := doRequest(client, http.MethodGet, baseURL+"/ops/kpi", "", nil)
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
}

func mustJSON(t *testing.T, client *http.Client, method, url, agentID string, body map[string]any) (int, []byte) {
	t.Helper()
	status, respBody, err := doRequest(client, method, url, agentID, body)
	if err != nil {
		t.Fatalf("%s %s request failed: %v", method, url, err)
	}
	return status, respBody
}

func doRequest(client *http.Client, method, url, agentID string, body map[string]any) (int, []byte, error) {
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
