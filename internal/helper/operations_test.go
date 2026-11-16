package helper

import "testing"

func TestOperationRegistry(t *testing.T) {
	for _, op := range []Op{OpIssue, OpRevoke, OpUnrevoke, OpReissue, OpTrash} {
		if !supportedOperation(op) {
			t.Fatalf("operation %q not registered", op)
		}
	}
	if supportedOperation(Op("delete")) {
		t.Fatal("unknown operation must not be supported")
	}
}
