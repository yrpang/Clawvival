package action

import (
	"reflect"
	"testing"
)

func TestActionHandlerInterface_DoesNotExposeBuildContext(t *testing.T) {
	typ := reflect.TypeOf((*ActionHandler)(nil)).Elem()
	if _, ok := typ.MethodByName("BuildContext"); ok {
		t.Fatal("ActionHandler should not expose BuildContext; context assembly belongs to UseCase pipeline")
	}
}
