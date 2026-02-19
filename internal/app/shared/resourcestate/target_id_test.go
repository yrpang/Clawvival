package resourcestate

import "testing"

func TestParseResourceTargetID(t *testing.T) {
	x, y, resource, ok := ParseResourceTargetID("res_-3_5_wood")
	if !ok {
		t.Fatalf("expected target id parse success")
	}
	if x != -3 || y != 5 || resource != "wood" {
		t.Fatalf("unexpected parse result: x=%d y=%d resource=%s", x, y, resource)
	}
}
