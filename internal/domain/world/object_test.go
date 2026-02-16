package world

import "testing"

func TestWorldObjectValidity(t *testing.T) {
	o := WorldObject{ID: "obj-1", Kind: ObjectFarm, Position: Point{X: 1, Y: 2}, HP: 10}
	if err := o.Validate(); err != nil {
		t.Fatalf("expected valid object, got %v", err)
	}

	bad := WorldObject{ID: "", Kind: ObjectFarm, HP: -1}
	if err := bad.Validate(); err == nil {
		t.Fatalf("expected invalid object")
	}
}
