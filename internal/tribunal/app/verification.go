package app

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/e3742526/tribunal/internal/tribunal/adapters"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
	"github.com/e3742526/tribunal/internal/tribunal/storage"
)

func (s *Service) verifyFindings(ctx context.Context, runDir string, findings []domain.Finding) ([]domain.EvidenceItem, []string, error) {
	allowedConfigured := map[string]bool{}
	for _, domainName := range s.Config.Workers.AllowedDomains {
		allowedConfigured[strings.ToLower(strings.TrimSuffix(domainName, "."))] = true
	}
	var evidence []domain.EvidenceItem
	var reasons []string
	tasks := 0
	for i := range findings {
		verified := false
		for _, reference := range findings[i].Evidence {
			if tasks >= s.Config.Limits.MaxVerification {
				reasons = append(reasons, "verification_cap_reached")
				break
			}
			target, err := s.resolveVerificationTarget(reference)
			if err != nil {
				continue
			}
			if !target.Builtin && !allowedConfigured[strings.ToLower(strings.TrimSuffix(target.Domain, "."))] {
				reasons = append(reasons, "evidence_domain_not_allowlisted")
				continue
			}
			allowed := append([]string{}, s.Config.Workers.AllowedDomains...)
			if target.Builtin {
				allowed = append(allowed, target.Domain)
			}
			headers := map[string]string{}
			if name := s.Config.Workers.WebSearchAuthEnv; target.Provider == "websearch" && name != "" && os.Getenv(name) != "" {
				headers["Authorization"] = "Bearer " + os.Getenv(name)
			}
			worker := adapters.WorkerService{AllowedDomains: allowed, MaxBytes: 2 << 20, Timeout: s.Config.Limits.CallTimeout, Clock: s.Clock, Headers: headers}
			item, fetchErr := worker.Fetch(ctx, target.URL, target.Provider, "post-review-verification")
			tasks++
			if fetchErr != nil {
				parsed, _ := url.Parse(target.URL)
				item = domain.EvidenceItem{SchemaVersion: 1, ID: fmt.Sprintf("evidence:failed-%03d", tasks), Task: target.Provider, Phase: "post-review-verification", Source: parsed.String(), RetrievedAt: s.now(), Status: "failed", Error: fetchErr.Error()}
				reasons = append(reasons, "verification_failed")
			} else {
				verified = true
			}
			evidence = append(evidence, item)
		}
		if verified {
			findings[i].EvidenceStatus = domain.EvidenceWorkerVerified
		}
	}
	payload := map[string]any{"schema_version": 1, "evidence": evidence, "verification_hash": hashText(string(marshal(evidence)))}
	if err := storage.WriteJSON(filepath.Join(runDir, "verification-evidence.json"), payload); err != nil {
		return nil, nil, err
	}
	return evidence, unique(reasons), nil
}

func (s *Service) resolveVerificationTarget(reference string) (adapters.EvidenceTarget, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(reference)), "search:") {
		return adapters.ResolveEvidenceTarget(reference)
	}
	if s.Config.Workers.WebSearchURL == "" {
		return adapters.EvidenceTarget{}, fmt.Errorf("generic web search endpoint is not configured")
	}
	endpoint, err := url.Parse(s.Config.Workers.WebSearchURL)
	if err != nil || endpoint.Scheme != "https" || endpoint.Hostname() == "" {
		return adapters.EvidenceTarget{}, fmt.Errorf("generic web search endpoint is invalid")
	}
	query := endpoint.Query()
	query.Set("q", strings.TrimSpace(reference[len("search:"):]))
	endpoint.RawQuery = query.Encode()
	return adapters.EvidenceTarget{URL: endpoint.String(), Provider: "websearch", Domain: endpoint.Hostname()}, nil
}
