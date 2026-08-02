package reconcile

import (
	"reflect"
	"testing"
)

func TestParseModelResponseValidatesAndSortsIDs(t *testing.T) {
	models, err := ParseModelResponse(`{"data":[{"id":"gpt-z"},{"id":"bad id"},{"id":"gpt-a"},{"id":"gpt-a"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"gpt-a", "gpt-z"}
	if !reflect.DeepEqual(models, want) {
		t.Fatalf("models=%v, want %v", models, want)
	}
}

func TestParseModelResponseRejectsEmptyData(t *testing.T) {
	if _, err := ParseModelResponse(`{"data":[]}`); err == nil {
		t.Fatal("empty model list was accepted")
	}
}
