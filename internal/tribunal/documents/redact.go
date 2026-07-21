package documents

import (
	"regexp"
	"sort"
	"strings"
)

type detection struct {
	start, end int
	class      string
}

var detectors = []struct {
	class string
	re    *regexp.Regexp
}{
	{"private-key", regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{"api-key", regexp.MustCompile(`(?i)(api[_-]?key|token|secret)\s*[:=]\s*[A-Za-z0-9_\-]{16,}`)},
	{"bearer-token", regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{16,}`)},
	{"email", regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)},
}

func scanAndRedact(item, content string, knownSecrets []string) (string, []Redaction) {
	var matches []detection
	for _, detector := range detectors {
		for _, loc := range detector.re.FindAllStringIndex(content, -1) {
			matches = append(matches, detection{loc[0], loc[1], detector.class})
		}
	}
	for _, secret := range knownSecrets {
		if len(secret) < 6 {
			continue
		}
		start := 0
		for {
			rel := strings.Index(content[start:], secret)
			if rel < 0 {
				break
			}
			at := start + rel
			matches = append(matches, detection{at, at + len(secret), "known-secret-value"})
			start = at + len(secret)
		}
	}
	if len(matches) == 0 {
		return content, nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start != matches[j].start {
			return matches[i].start < matches[j].start
		}
		return matches[i].end > matches[j].end
	})
	merged := matches[:0]
	for _, match := range matches {
		if len(merged) > 0 && match.start < merged[len(merged)-1].end {
			if match.end > merged[len(merged)-1].end {
				merged[len(merged)-1].end = match.end
			}
			continue
		}
		merged = append(merged, match)
	}
	data := []byte(content)
	redactions := make([]Redaction, 0, len(merged))
	for _, match := range merged {
		for i := match.start; i < match.end; i++ {
			if data[i] != '\n' && data[i] != '\r' {
				data[i] = '*'
			}
		}
		redactions = append(redactions, Redaction{SchemaVersion: 1, PacketItem: item, Start: match.start, End: match.end, Class: match.class, Reason: "sensitive input redacted before model delivery"})
	}
	return string(data), redactions
}
