package tagteam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvocationStreamRedactsSecretAcrossWriteBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout.txt")
	stream, err := newInvocationStream(path, 1024, map[string]string{"TAGTEAM_SECRET_TOKEN": "split-secret"})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = stream.Write([]byte("prefix split-"))
	_, _ = stream.Write([]byte("secret suffix"))
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "split-secret") || !strings.Contains(string(data), redactedSecret) {
		t.Fatalf("persisted stream was not redacted: %q", data)
	}
}

func TestInvocationStreamPersistsBoundedPrefixAndMarksTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stderr.txt")
	stream, err := newInvocationStream(path, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = stream.Write([]byte("123456789"))
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "12345" || !stream.Exceeded() || stream.Received() != 9 {
		t.Fatalf("data=%q exceeded=%t received=%d", data, stream.Exceeded(), stream.Received())
	}
}
