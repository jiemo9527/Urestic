package httpapi

import (
	"reflect"
	"testing"
)

func TestForgetSnapshotArgsPrunesUnreferencedData(t *testing.T) {
	got := forgetSnapshotArgs("s3:https://example.com/bucket", "deadbeef")
	want := []string{"restic", "-r", "s3:https://example.com/bucket", "forget", "--prune", "deadbeef"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected restic command arguments: %q", got)
	}
}
