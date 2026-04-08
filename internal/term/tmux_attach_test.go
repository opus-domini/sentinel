package term

import (
	"reflect"
	"testing"
)

func TestTmuxAttachArgs(t *testing.T) {
	t.Parallel()

	got := tmuxAttachArgs("dev")
	want := []string{"attach", "-f", tmuxAttachClientFlags, "-t", "dev"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tmuxAttachArgs() = %v, want %v", got, want)
	}
}
