package world

import "errors"

type ObjectKind string

const (
	ObjectBed     ObjectKind = "bed"
	ObjectBox     ObjectKind = "box"
	ObjectFarm    ObjectKind = "farm"
	ObjectTorch   ObjectKind = "torch"
	ObjectWall    ObjectKind = "wall"
	ObjectDoor    ObjectKind = "door"
	ObjectFurnace ObjectKind = "furnace"
)

type WorldObject struct {
	ID       string
	Kind     ObjectKind
	Position Point
	HP       int
}

var ErrInvalidWorldObject = errors.New("invalid world object")

func (o WorldObject) Validate() error {
	if o.ID == "" || o.Kind == "" || o.HP < 0 {
		return ErrInvalidWorldObject
	}
	return nil
}
