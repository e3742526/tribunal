package tagteam

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "tagteam-test-state-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("TAGTEAM_STATE_ROOT", root)
	code := m.Run()
	_ = os.RemoveAll(root)
	os.Exit(code)
}
