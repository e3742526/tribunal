package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/golangci/misspell"

	"github.com/e3742526/tribunal/internal/tribunal/documents"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

type WorkerService struct {
	AllowedDomains      []string
	MaxBytes            int64
	Timeout             time.Duration
	Client              *http.Client
	AllowPrivateForTest bool
	Clock               func() time.Time
}

func (w *WorkerService) Fetch(ctx context.Context, rawURL, task, phase string) (domain.EvidenceItem, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		if !(w.AllowPrivateForTest && parsed != nil && parsed.Scheme == "http") {
			return domain.EvidenceItem{}, fmt.Errorf("worker URL must be an absolute HTTPS URL")
		}
	}
	if !exactDomainAllowed(parsed.Hostname(), w.AllowedDomains) {
		return domain.EvidenceItem{}, fmt.Errorf("worker domain %q is not allowlisted", parsed.Hostname())
	}
	if !w.AllowPrivateForTest {
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, parsed.Hostname())
		if err != nil {
			return domain.EvidenceItem{}, fmt.Errorf("resolve worker domain: %w", err)
		}
		for _, address := range addresses {
			if address.IP.IsPrivate() || address.IP.IsLoopback() || address.IP.IsLinkLocalUnicast() || address.IP.IsLinkLocalMulticast() || address.IP.IsUnspecified() {
				return domain.EvidenceItem{}, fmt.Errorf("worker domain resolves to a private or local address")
			}
		}
	}
	timeout := w.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return domain.EvidenceItem{}, err
	}
	client := w.Client
	if client == nil {
		client = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !exactDomainAllowed(req.URL.Hostname(), w.AllowedDomains) {
				return fmt.Errorf("redirect escaped domain allowlist")
			}
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return domain.EvidenceItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return domain.EvidenceItem{}, fmt.Errorf("worker fetch returned status %d", resp.StatusCode)
	}
	limit := w.MaxBytes
	if limit <= 0 {
		limit = 2 << 20
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return domain.EvidenceItem{}, err
	}
	if int64(len(data)) > limit {
		return domain.EvidenceItem{}, fmt.Errorf("worker response exceeded %d bytes", limit)
	}
	clock := w.Clock
	if clock == nil {
		clock = time.Now
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	excerpt := strings.TrimSpace(string(data))
	if len(excerpt) > 4000 {
		excerpt = excerpt[:4000]
	}
	return domain.EvidenceItem{SchemaVersion: 1, ID: "evidence:" + hash[:12], Task: task, Phase: phase, Source: parsed.String(), RetrievedAt: clock().UTC(), Excerpt: excerpt, ContentSHA256: hash, Status: "ok"}, nil
}

func exactDomainAllowed(host string, allowed []string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, candidate := range allowed {
		if host == strings.ToLower(strings.TrimSuffix(candidate, ".")) {
			return true
		}
	}
	return false
}

func Spellcheck(packet documents.Packet) []domain.Finding {
	replacer := misspell.New()
	var findings []domain.Finding
	for _, item := range packet.Items {
		_, differences := replacer.Replace(item.Content)
		searchAt := 0
		for _, difference := range differences {
			rel := strings.Index(item.Content[searchAt:], difference.Original)
			if rel < 0 {
				continue
			}
			start := searchAt + rel
			end := start + len(difference.Original)
			id := fmt.Sprintf("W-SPELL-%03d", len(findings)+1)
			findings = append(findings, domain.Finding{SchemaVersion: domain.FindingSchemaVersion, ID: id, Reviewer: "worker/spellcheck", Origin: "worker", Severity: domain.SeverityNit, Category: domain.CategoryStyle, Anchor: domain.Anchor{Kind: "quote", PacketItem: item.ID, Quote: difference.Original, CharOffset: start, EndOffset: end, ItemSHA256: item.PacketSHA256}, Issue: fmt.Sprintf("Possible misspelling %q", difference.Original), Recommendation: fmt.Sprintf("Consider %q", difference.Corrected), EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"})
			searchAt = end
		}
	}
	return findings
}

var numericCitation = regexp.MustCompile(`\[([0-9]{1,4})\]`)
var numericReference = regexp.MustCompile(`(?m)^\s*\[([0-9]{1,4})\]\s+`)

func ReferenceCheck(packet documents.Packet) []domain.Finding {
	var findings []domain.Finding
	for _, item := range packet.Items {
		definitions := map[string]bool{}
		for _, match := range numericReference.FindAllStringSubmatch(item.Content, -1) {
			definitions[match[1]] = true
		}
		seen := map[string]bool{}
		for _, loc := range numericCitation.FindAllStringSubmatchIndex(item.Content, -1) {
			key := item.Content[loc[2]:loc[3]]
			if definitions[key] || seen[key] {
				continue
			}
			seen[key] = true
			quote := item.Content[loc[0]:loc[1]]
			findings = append(findings, domain.Finding{SchemaVersion: 2, ID: fmt.Sprintf("W-REF-%03d", len(findings)+1), Reviewer: "worker/refcheck", Origin: "worker", Severity: domain.SeverityMinor, Category: domain.CategoryCitationIntegrity, Anchor: domain.Anchor{Kind: "quote", PacketItem: item.ID, Quote: quote, CharOffset: loc[0], EndOffset: loc[1], ItemSHA256: item.PacketSHA256}, Issue: fmt.Sprintf("Citation %s has no matching numbered reference", quote), Recommendation: "Add the corresponding reference or remove the citation", EvidenceStatus: domain.EvidenceAnchored, Confidence: "high"})
		}
	}
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	return findings
}
