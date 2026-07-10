package tagteam

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
)

// invocationStream persists bounded, redacted subprocess output while also
// retaining the same bytes for adapter parsing. It holds a small suffix so a
// secret split across Write calls is still redacted before reaching disk.
type invocationStream struct {
	mu        sync.Mutex
	file      *os.File
	path      string
	buffer    bytes.Buffer
	pending   []byte
	secrets   []string
	maxSecret int
	limit     int64
	received  int64
	written   int64
	exceeded  bool
	closed    bool
}

func newInvocationStream(path string, limit int64, overlay map[string]string) (*invocationStream, error) {
	if limit <= 0 {
		limit = 2 * 1024 * 1024
	}
	if err := os.MkdirAll(filepathDir(path), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	secrets := secretValuesFromEnv(overlay)
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	maxSecret := 0
	for _, secret := range secrets {
		if len(secret) > maxSecret {
			maxSecret = len(secret)
		}
	}
	return &invocationStream{file: file, path: path, limit: limit, secrets: secrets, maxSecret: maxSecret}, nil
}

func filepathDir(path string) string {
	index := strings.LastIndexAny(path, `/\\`)
	if index < 0 {
		return "."
	}
	if index == 0 {
		return path[:1]
	}
	return path[:index]
}

func (s *invocationStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, os.ErrClosed
	}
	s.received += int64(len(p))
	combined := append(append([]byte(nil), s.pending...), p...)
	cut := len(combined)
	if s.maxSecret > 1 && cut >= s.maxSecret {
		cut -= s.maxSecret - 1
	} else if s.maxSecret > 1 {
		cut = 0
	}
	cut = s.safeRedactionCut(combined, cut)
	if cut > 0 {
		s.persist(redactKnownSecrets(string(combined[:cut]), s.secrets))
	}
	s.pending = append(s.pending[:0], combined[cut:]...)
	return len(p), nil
}

func (s *invocationStream) safeRedactionCut(data []byte, cut int) int {
	if cut <= 0 || len(s.secrets) == 0 {
		return cut
	}
	text := string(data)
	for _, secret := range s.secrets {
		searchAt := 0
		for {
			index := strings.Index(text[searchAt:], secret)
			if index < 0 {
				break
			}
			index += searchAt
			if index < cut && index+len(secret) > cut {
				cut = index
			}
			searchAt = index + 1
		}
	}
	return cut
}

func redactKnownSecrets(text string, secrets []string) string {
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, redactedSecret)
	}
	return text
}

func (s *invocationStream) persist(text string) {
	if text == "" {
		return
	}
	remaining := s.limit - s.written
	if remaining <= 0 {
		s.exceeded = true
		return
	}
	data := []byte(text)
	if int64(len(data)) > remaining {
		data = data[:remaining]
		s.exceeded = true
	}
	_, _ = s.file.Write(data)
	_, _ = s.buffer.Write(data)
	s.written += int64(len(data))
}

func (s *invocationStream) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.file.Sync()
}

func (s *invocationStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	if len(s.pending) > 0 {
		s.persist(redactKnownSecrets(string(s.pending), s.secrets))
		s.pending = nil
	}
	s.closed = true
	if err := s.file.Sync(); err != nil {
		_ = s.file.Close()
		return err
	}
	return s.file.Close()
}

func (s *invocationStream) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.buffer.Bytes()...)
}

func (s *invocationStream) String() string { return string(s.Bytes()) }

func (s *invocationStream) Exceeded() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exceeded || s.received > s.limit
}

func (s *invocationStream) Received() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.received
}

func (s *invocationStream) Written() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.written
}

func (s *invocationStream) describe() string {
	return fmt.Sprintf("%s bytes=%d", s.path, s.Written())
}
