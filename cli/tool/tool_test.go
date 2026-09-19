package tool

import (
	"strings"
	"testing"
)

// A missing binary is nearly always a missing pin or the wrong directory, so
// the error is the line to add rather than exec's "file not found in $PATH".
func TestMissingBinaryNamesThePin(t *testing.T) {
	pin := `"go:example.com/cmd/nope" = "v1.0.0"`
	_, err := Run("a-binary-nothing-has", pin, "--version")
	if err == nil {
		t.Fatal("no error for a binary that does not exist")
	}
	if !strings.Contains(err.Error(), pin) {
		t.Errorf("err = %v; want it to quote the mise line to add", err)
	}
}

// A checker exits non-zero because it found something. That is its answer,
// not a failure to run, so what it printed comes back.
func TestNonZeroWithOutputIsStillAnAnswer(t *testing.T) {
	res, err := Run("sh", "(no pin)", "-c", "echo '{\"found\":1}'; exit 1")
	if err != nil {
		t.Fatalf("err = %v; want the output back", err)
	}
	if !strings.Contains(res.Out, `"found":1`) {
		t.Errorf("out = %q; want what it printed", res.Out)
	}
	// Every run is timed, which is why a caller can say which tool was slow.
	if res.Took <= 0 {
		t.Error("the run reports no duration")
	}
	// And it decodes through the generic method, typed by the caller.
	type found struct {
		Found int `json:"found"`
	}
	got, err := res.JSON[found]("the tool's answer")
	if err != nil || got.Found != 1 {
		t.Errorf("JSON = %+v, %v; want found 1", got, err)
	}
}

// Nothing printed and a failure is a real failure.
func TestNonZeroWithNoOutputIsAFailure(t *testing.T) {
	if _, err := Run("sh", "(no pin)", "-c", "exit 3"); err == nil {
		t.Error("no error for a tool that printed nothing and failed")
	}
}
