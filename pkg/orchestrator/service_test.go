package orchestrator

import (
	"reflect"
	"testing"

	"github.com/lyzrai/flow/pkg/models"
)

func TestStepResult_roundTrip(t *testing.T) {
	src := map[int][]models.Item{
		0: {{"a": 1}, {"a": 2}},
		1: {{"b": "x"}},
	}
	got := fromStepResult(toStepResult(src))
	if !reflect.DeepEqual(got, src) {
		t.Fatalf("round trip mismatch:\n got: %+v\n src: %+v", got, src)
	}
}

func TestStepResult_emptyInput(t *testing.T) {
	got := fromStepResult(toStepResult(map[int][]models.Item{}))
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestToStepResult_keysAreStringIndexed(t *testing.T) {
	got := toStepResult(map[int][]models.Item{0: {{"k": 1}}, 2: {{"k": 2}}})
	if _, ok := got.Outputs["0"]; !ok {
		t.Error("missing key 0")
	}
	if _, ok := got.Outputs["2"]; !ok {
		t.Error("missing key 2")
	}
	if _, ok := got.Outputs["1"]; ok {
		t.Error("did not expect key 1 (skipped index)")
	}
}
