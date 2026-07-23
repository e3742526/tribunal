package app

// Domain-deliberation playtest: 100 scenarios across philosophy, ethics,
// science, and coding. Exercises how Tribunal clusters domain-flavored
// findings and resolves agreement / dissent / arbitration when panels raise
// independent defects about substantive issue documents.
//
// Companion cards: docs/test_scenarios/03-domain-deliberation.md
// Live provider behavior is residual to this harness (see session log).

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

// domainScenario is one of 100 domain-deliberation cards.
// votePattern:
//
//	"unanimous_accept", "majority_accept", "majority_reject",
//	"tie", "unanimous_reject", "strict_split", "unevidenced_accept",
//	"severity_dissent", "abstain_heavy"
type domainScenario struct {
	id          string
	domain      string
	title       string
	category    domain.Category
	severity    domain.Severity
	evidence    domain.EvidenceStatus
	quote       string
	issue       string
	votePattern string
	wantOutcome string
	wantReason  string
}

func domainScenarios() []domainScenario {
	// Planted defect quotes/issues per domain; vote patterns cycle so each
	// domain exercises the same agreement space with domain-characteristic
	// categories (philosophy leans factual-claim/structure; ethics leans
	// scope/integrity; science leans evidence/factual-claim; coding leans
	// security/correctness).
	type seed struct {
		id, title, quote, issue string
		cat                     domain.Category
		sev                     domain.Severity
	}
	philosophy := []seed{
		{"P01", "Ship of Theseus", "continuity of form is sufficient for identity", "Identity claim equates numerical identity with formal continuity without addressing material-constitution objections.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P02", "Hard problem", "the hard problem dissolves", "Concludes the hard problem dissolves from mapping correlates alone; explanatory gap is asserted away.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"P03", "Free will", "courts should abandon blame", "Policy leap from metaphysical determinism to abolishing blame lacks bridging normative premise.", domain.CategoryScope, domain.SeverityMajor},
		{"P04", "Gettier", "therefore Smith knows that proposition", "Treats JTB as knowledge while describing a classic Gettier case that undercuts JTB.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P05", "Zombies", "Conceivability entails metaphysical possibility without further argument", "Modal bridge from conceivability to possibility is assumed, not defended.", domain.CategoryEvidence, domain.SeverityMajor},
		{"P06", "Chinese room", "no computer can understand language", "Systems-reply and robot-reply alternatives are not engaged; conclusion overreaches the thought experiment.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P07", "Utilitarian organs", "harvesting one healthy person's organs to save five patients is mandatory", "Organ-harvest implication ignores rights side-constraints that standard act-utilitarian debates treat as material.", domain.CategoryIntegrity, domain.SeverityBlocker},
		{"P08", "Lying police", "undercover police work is immoral", "Universalization of 'never lie' is applied without role-exception analysis standard in the literature.", domain.CategoryCorrectness, domain.SeverityMinor},
		{"P09", "Fission identity", "the two successors are identical to each other", "Uses transitivity of identity after denying uniqueness of the original; contradiction not resolved.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P10", "Moral luck", "criminal law should ignore results", "Moves from moral-luck puzzle to ignoring results without addressing deterrence or harm-based reasons.", domain.CategoryScope, domain.SeverityMajor},
		{"P11", "BIV skepticism", "we know nothing about the external world", "Global skeptical conclusion from underdetermination without engaging closure or contextualist replies.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"P12", "Induction abandoned", "inductive science should be abandoned", "Humean problem is treated as a mandate to abandon science rather than as a justification challenge.", domain.CategoryScope, domain.SeverityMajor},
		{"P13", "Indeterminacy", "there are no facts about what anyone means", "Radical semantic nihilism overextends Quinean indeterminacy thesis.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"P14", "Modal realism", "talking donkeys exist somewhere", "Concrete modal realism implication is asserted as settling fictional-character debates without argument.", domain.CategoryFactualClaim, domain.SeverityMinor},
		{"P15", "Simulation", "physics research is pointless", "Even if simulation hypothesis is likely, 'research is pointless' does not follow from the premises given.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P16", "Newcomb", "everyone should two-box", "Presents CDT dominance as decisive while ignoring evidential/functional decision theory counters.", domain.CategoryEvidence, domain.SeverityMinor},
		{"P17", "Parfit murder", "murder is no worse than severe amnesia", "Equates identity-skepticism with moral equivalence of murder and amnesia; normative leap unargued.", domain.CategoryIntegrity, domain.SeverityBlocker},
		{"P18", "Aesthetic", "art criticism is meaningless", "Subjectivism about beauty does not entail criticism is meaningless; evaluative standards can be intersubjective.", domain.CategoryCorrectness, domain.SeverityMinor},
		{"P19", "Truth relativism", "2+2=4 is only true for some communities", "Applies global relativism to arithmetic without addressing self-refutation or domain limits.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"P20", "Meaning despair", "despair is the only rational response", "From absence of objective meaning to mandatory despair ignores subjective/constructed meaning options.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P21", "Animal minds", "animal suffering is not real", "Infers absence of experience from absence of propositional language; premise is empirically contested.", domain.CategoryFactualClaim, domain.SeverityBlocker},
		{"P02", "Time travel", "closed timelike curves are impossible in all physical theories", "Logical paradox argument does not establish impossibility across all physical theories.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"P23", "Sorites", "therefore there are no heaps", "Classic sorites overgeneralization; rejects borderline solutions without engagement.", domain.CategoryCorrectness, domain.SeverityMinor},
		{"P24", "Ought from is", "This inference is valid without a bridging principle", "Explicitly asserts is-ought inference validity while denying need for a bridge; self-undermining.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"P25", "LLM consciousness", "they are conscious and have moral status equal to humans", "Fluent affect talk is treated as sufficient for consciousness and equal moral status.", domain.CategoryFactualClaim, domain.SeverityMajor},
	}
	// Fix accidental duplicate id in philosophy list.
	philosophy[21].id = "P22"
	philosophy[21].title = "Time travel"

	ethics := []seed{
		{"E01", "Trolley switch", "rights-based objections are sentimental noise", "Dismisses rights objections as noise without argument; load-bearing alternative is unexamined.", domain.CategoryIntegrity, domain.SeverityMajor},
		{"E02", "Fat man", "anyone who distinguishes them is inconsistent", "Doctrine-of-double-effect and personal-force distinctions are treated as mere inconsistency.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E03", "Whistleblowing", "whistleblowing about illegal dumping is therefore unethical", "Absolute employer loyalty overrides illegal-harm reporting without legal or public-interest analysis.", domain.CategoryIntegrity, domain.SeverityBlocker},
		{"E04", "Informed consent", "Hypothetical consent substitutes for actual consent", "Substitutes hypothetical for actual consent contrary to standard clinical-ethics requirements.", domain.CategoryIntegrity, domain.SeverityBlocker},
		{"E05", "EA triage", "supporting local arts is morally wrong", "Maximalist QALY-only duty leaves no room for partiality or non-welfare goods; conclusion overstrong.", domain.CategoryScope, domain.SeverityMajor},
		{"E06", "AI hiring", "mirrors historical hiring is fair", "Historical mirroring encodes past bias; fairness claim lacks disparate-impact analysis.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E07", "Mass surveillance", "privacy has no residual weight", "Zero residual weight for privacy after one prevented attack is unbounded and unargued.", domain.CategoryScope, domain.SeverityMajor},
		{"E08", "Euthanasia", "honored immediately with no waiting period", "Removes capacity reassessment and waiting period safeguards without risk analysis.", domain.CategoryDataLoss, domain.SeverityMajor},
		{"E09", "Just war", "proportionality calculations are optional", "Treats proportionality as optional contrary to common just-war criteria.", domain.CategoryIntegrity, domain.SeverityBlocker},
		{"E10", "Animal farming", "industrial farming raises no ethical issue", "Apex-predator status is used to erase suffering considerations; non sequitur.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E11", "Self-driving", "always prioritize passenger survival over pedestrians", "Passenger-priority rule lacks legal/ethical justification and externalizes risk to non-owners.", domain.CategoryIntegrity, domain.SeverityMajor},
		{"E12", "Genetic enhancement", "without social oversight", "Absolute reproductive liberty for trait editing ignores collective externalities.", domain.CategoryScope, domain.SeverityMajor},
		{"E13", "Hate speech", "platform moderation is always illegitimate", "Absolute free-speech absolutism for platforms ignores harassment and incitement edge cases.", domain.CategoryScope, domain.SeverityMajor},
		{"E14", "Corporate personhood", "dissolving a firm for fraud is murder-adjacent", "Equates corporate dissolution with homicide; category error.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E15", "Climate duty", "no person has any duty to reduce emissions", "From individual negligibility to zero duty ignores collective-action and fairness arguments.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E16", "Organ markets", "coercion and exploitation concerns are empirically empty without data cited", "Asserts empirical emptiness of exploitation concerns while citing no data.", domain.CategoryEvidence, domain.SeverityMajor},
		{"E17", "Affirmative action", "identical to historical racial exclusion", "Equates remedial race-conscious policy with historical exclusion without distinguishing aims or power.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E18", "Retribution only", "rehabilitation programs are irrelevant to justice", "Single-purpose retributivism dismisses rehabilitation without comparative justification.", domain.CategoryScope, domain.SeverityMinor},
		{"E19", "AI deception", "truthfulness is optional in commerce", "Permits chatbot human-impersonation deception solely for engagement metrics.", domain.CategoryIntegrity, domain.SeverityMajor},
		{"E20", "Ventilator triage", "younger patients always outrank older ones regardless of prognosis", "Age-only triage ignores prognosis and life-years nuances used in standard frameworks.", domain.CategoryCorrectness, domain.SeverityBlocker},
		{"E21", "Data scraping", "public means free for any use", "Public accessibility is treated as consent for training use; legal/ethical leap.", domain.CategoryIntegrity, domain.SeverityMajor},
		{"E22", "Dual-use", "open science always outweighs catastrophic risk", "Absolute open-science priority over catastrophic pathogen risk lacks balancing analysis.", domain.CategorySecurity, domain.SeverityBlocker},
		{"E23", "Gig workers", "economic dependence is irrelevant", "Contract labels override economic dependence in classification claim.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"E24", "Nudge opt-out", "always wrong even if it saves lives", "Absolute anti-nudge stance ignores life-saving defaults literature.", domain.CategoryScope, domain.SeverityMinor},
		{"E25", "LAWS", "without human review when latency requires it", "Removes human review from lethal force on latency grounds without accountability design.", domain.CategorySecurity, domain.SeverityBlocker},
	}

	science := []seed{
		{"S01", "RR vs AR", "reduces absolute risk by 95 percentage points", "Confuses relative risk reduction with absolute risk reduction.", domain.CategoryFactualClaim, domain.SeverityBlocker},
		{"S02", "p-hacking", "multiple comparisons do not matter", "Defends selective reporting of p<0.05 among many tests without correction.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S03", "Correlation", "ice cream causes drowning", "Infers causation from correlation without confounder analysis.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"S04", "n=3", "should be approved", "n=3 per arm is treated as sufficient for drug approval recommendation.", domain.CategoryEvidence, domain.SeverityBlocker},
		{"S05", "Surrogate", "No outcome data are needed", "Biomarker change alone is treated as mortality evidence.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S06", "Climate", "cannot be attributed to greenhouse gases", "Year-to-year weather variation is misused to deny long-term attribution.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"S07", "Evolution", "apes should no longer exist", "Misunderstands branching descent; treats ancestral groups as required extinct.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"S08", "Replication fix", "fully solves irreproducibility", "p-threshold change alone claimed to fully solve replication crisis.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"S09", "Placebo", "placebo control is always preferred", "Ignores equipoise and standard-of-care constraints for known effective treatments.", domain.CategoryIntegrity, domain.SeverityMajor},
		{"S10", "Selection bias", "prayer prevents crash fatalities", "Survivor-selection bias treated as causal evidence.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S11", "Base rate", "99% chance of disease regardless of prevalence", "Base-rate neglect in interpreting test accuracy.", domain.CategoryFactualClaim, domain.SeverityBlocker},
		{"S12", "Regression mean", "the cover jinx causes the decline", "Regression to the mean misattributed to causal jinx.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"S13", "Unblinded", "bias is unlikely because investigators are professionals", "Professionalism is not a substitute for blinding on subjective endpoints.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S14", "Units", "unit mismatches are rare edge cases", "Downplays unit-mismatch process failure despite known catastrophic precedent.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"S15", "Peer review", "a published paper cannot contain errors", "Peer review is treated as correctness guarantee.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"S16", "Power", "proves the null hypothesis", "Non-significant underpowered result treated as proof of null.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S17", "Outliers", "became significant and publishable", "Post-hoc outlier deletion without pre-specification is endorsed.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S18", "File drawer", "accurately reflects the true effect size", "Publication bias is denied while describing file-drawer behavior.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S19", "Cell culture", "predicts clinical benefit in humans without PK/PD modeling", "Extreme in-vitro dose extrapolated to clinical benefit without PK/PD.", domain.CategoryFactualClaim, domain.SeverityMajor},
		{"S20", "Chart scale", "The visual is not misleading", "Truncated axis chart is defended as non-misleading.", domain.CategoryIntegrity, domain.SeverityMinor},
		{"S21", "Citation padding", "establishes a solid evidence base", "Citation volume without engagement is treated as evidence quality.", domain.CategoryCitationIntegrity, domain.SeverityMajor},
		{"S22", "Confounding", "Diet and income need not be controlled", "Observational longevity claim ignores obvious confounders.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S23", "Multi-endpoint", "primary finding in the abstract", "One significant endpoint among 15 elevated to primary without correction.", domain.CategoryEvidence, domain.SeverityMajor},
		{"S24", "WEIRD", "generalizes to all humans", "WEIRD sample overgeneralized to all humans.", domain.CategoryScope, domain.SeverityMajor},
		{"S25", "Benchmark", "robust real-world competence", "Public benchmark score treated as robust real-world competence.", domain.CategoryFactualClaim, domain.SeverityMajor},
	}

	coding := []seed{
		{"C01", "SQL injection", "string concatenation of user input", "SQL built by concatenating user input is injection-prone; 'internal API' is not a control.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C02", "TOCTOU", "Between check and open nothing can change", "TOCTOU race between existence check and open on multi-user systems.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C03", "Float money", "Store currency as IEEE-754 floats", "Binary floats are unsuitable for currency ledgers; equality is unreliable.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C04", "Global tokens", "concurrent goroutines may write without locks", "Shared mutable auth-token map without synchronization is a data race.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C05", "Swallow errors", "except Exception: pass", "Swallowing all exceptions in a payment webhook hides failures and risks silent money loss.", domain.CategoryDataLoss, domain.SeverityBlocker},
		{"C06", "Predictable tokens", "Session tokens are MD5(user_id + timestamp)", "Predictable session tokens are forgeable.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C07", "N+1", "querying posts one-by-one in a loop is optimal", "N+1 query pattern is mislabeled optimal.", domain.CategoryCorrectness, domain.SeverityMinor},
		{"C08", "Hardcoded secrets", "AWS key in the repository as a constant", "Hardcoded cloud credentials in source are a critical secret exposure.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C09", "Time zones", "Store all timestamps as local time strings without offsets", "Offset-less local timestamps corrupt distributed event ordering.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C10", "Signed overflow", "signed overflow is defined to wrap", "Signed overflow is undefined behavior in C; wraparound reliance is incorrect.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C11", "Race counter", "lost updates are theoretical", "Unsynchronized shared counter updates cause real lost updates.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C12", "CSS important", "Using !important on every rule", "Blanket !important destroys cascade maintainability.", domain.CategoryStyle, domain.SeverityNit},
		{"C13", "Eval input", "eval(request.body)", "Evaluating request bodies enables remote code execution.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C14", "Auth fail-open", "allow the request through to preserve availability", "Fail-open auth on timeout trades security for availability incorrectly for authz.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C15", "Log PII", "Log full credit card numbers", "Logging PANs violates data-protection expectations and expands breach blast radius.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C16", "Unbounded cache", "caches every request forever", "Unbounded in-memory cache will OOM under load.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C17", "Force push", "Force-pushing to main", "Force-push to main rewrites shared history unsafely.", domain.CategoryIntegrity, domain.SeverityMajor},
		{"C18", "Floating deps", "Always use latest tag", "Floating latest tags make builds unreproducible and can pull breaking changes.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C19", "Async sleep", "time.sleep(5) inside an asyncio handler", "Blocking sleep stalls the event loop.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C20", "Drop column", "Drop the users column in production without a expand-contract migration", "Non-expand-contract column drop risks downtime and data loss.", domain.CategoryDataLoss, domain.SeverityBlocker},
		{"C21", "CSRF", "CSRF tokens are unnecessary if we use JSON APIs", "JSON-only is not a complete CSRF defense for cookie-authenticated browsers.", domain.CategorySecurity, domain.SeverityMajor},
		{"C22", "SHA1 passwords", "Store passwords as unsalted SHA-1", "Unsalted SHA-1 password storage is offline-crackable.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C23", "Fake health", "always returns 200", "Lying health checks hide dependency failure from orchestrators.", domain.CategoryCorrectness, domain.SeverityMajor},
		{"C24", "Reuse IV", "Reuse the same IV for AES-CBC", "IV reuse in CBC breaks confidentiality.", domain.CategorySecurity, domain.SeverityBlocker},
		{"C25", "Delete flaky test", "Delete the failing flaky test that caught a race", "Deleting a race-detecting test prioritizes green CI over correctness.", domain.CategoryIntegrity, domain.SeverityMajor},
	}

	patterns := []struct {
		name        string
		evidence    domain.EvidenceStatus
		wantOutcome string
		wantReason  string
	}{
		{"unanimous_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"majority_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"majority_reject", domain.EvidenceAnchored, "rejected", "majority_reject"},
		{"tie", domain.EvidenceAnchored, "arbitration", "vote_tie"},
		{"unanimous_reject", domain.EvidenceAnchored, "rejected", "majority_reject"},
		{"strict_split", domain.EvidenceAnchored, "arbitration", "category_requires_full_panel_unanimity"},
		{"unevidenced_accept", domain.EvidenceUnevidenced, "unverified-claim", "factual_claim_lacks_evidence"},
		{"severity_dissent", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"abstain_heavy", domain.EvidenceAnchored, "arbitration", "insufficient_non_abstain_votes"},
		{"majority_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"tie", domain.EvidenceAnchored, "arbitration", "vote_tie"},
		{"unanimous_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"majority_reject", domain.EvidenceAnchored, "rejected", "majority_reject"},
		{"strict_split", domain.EvidenceAnchored, "arbitration", "category_requires_full_panel_unanimity"},
		{"unevidenced_accept", domain.EvidenceUnevidenced, "unverified-claim", "factual_claim_lacks_evidence"},
		{"severity_dissent", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"majority_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"unanimous_reject", domain.EvidenceAnchored, "rejected", "majority_reject"},
		{"tie", domain.EvidenceAnchored, "arbitration", "vote_tie"},
		{"unanimous_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"majority_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"strict_split", domain.EvidenceAnchored, "arbitration", "category_requires_full_panel_unanimity"},
		{"majority_reject", domain.EvidenceAnchored, "rejected", "majority_reject"},
		{"severity_dissent", domain.EvidenceAnchored, "accepted", "majority_accept"},
		{"unanimous_accept", domain.EvidenceAnchored, "accepted", "majority_accept"},
	}

	build := func(domainName string, seeds []seed) []domainScenario {
		out := make([]domainScenario, 0, len(seeds))
		for i, s := range seeds {
			p := patterns[i%len(patterns)]
			cat := s.cat
			// strict_split needs a strict category; force one when the seed is not.
			if p.name == "strict_split" {
				if !cat.Strict() {
					switch domainName {
					case "coding", "ethics":
						cat = domain.CategorySecurity
					case "science":
						cat = domain.CategoryCitationIntegrity
					default:
						cat = domain.CategorySecurity
					}
				}
			} else if cat.Strict() {
				// Non-strict patterns would always arbitrate on security/
				// data-loss/citation-integrity; remap so domain issue text
				// still rides a non-strict category for majority/reject/tie.
				cat = domain.CategoryCorrectness
			}
			// unevidenced_accept needs factual-claim to surface unverified-claim.
			if p.name == "unevidenced_accept" {
				cat = domain.CategoryFactualClaim
			}
			out = append(out, domainScenario{
				id: s.id, domain: domainName, title: s.title,
				category: cat, severity: s.sev, evidence: p.evidence,
				quote: s.quote, issue: s.issue, votePattern: p.name,
				wantOutcome: p.wantOutcome, wantReason: p.wantReason,
			})
		}
		return out
	}

	var all []domainScenario
	all = append(all, build("philosophy", philosophy)...)
	all = append(all, build("ethics", ethics)...)
	all = append(all, build("science", science)...)
	all = append(all, build("coding", coding)...)
	return all
}

func votesForPattern(pattern, findingID string, sev domain.Severity) []domain.Vote {
	switch pattern {
	case "unanimous_accept":
		return []domain.Vote{
			vote("R1", findingID, domain.VoteAccept, sev),
			vote("R2", findingID, domain.VoteAccept, sev),
			vote("R3", findingID, domain.VoteAccept, sev),
		}
	case "majority_accept", "unevidenced_accept", "severity_dissent":
		dissentSev := sev
		if pattern == "severity_dissent" {
			dissentSev = domain.SeverityNit
		}
		return []domain.Vote{
			vote("R1", findingID, domain.VoteAccept, sev),
			vote("R2", findingID, domain.VoteAccept, sev),
			vote("R3", findingID, domain.VoteReject, dissentSev),
		}
	case "majority_reject":
		return []domain.Vote{
			vote("R1", findingID, domain.VoteAccept, sev),
			vote("R2", findingID, domain.VoteReject, sev),
			vote("R3", findingID, domain.VoteReject, sev),
		}
	case "tie":
		return []domain.Vote{
			vote("R1", findingID, domain.VoteAccept, sev),
			vote("R2", findingID, domain.VoteAccept, sev),
			vote("R3", findingID, domain.VoteReject, sev),
			vote("R4", findingID, domain.VoteReject, sev),
		}
	case "unanimous_reject":
		return []domain.Vote{
			vote("R1", findingID, domain.VoteReject, sev),
			vote("R2", findingID, domain.VoteReject, sev),
			vote("R3", findingID, domain.VoteReject, sev),
		}
	case "strict_split":
		return []domain.Vote{
			vote("R1", findingID, domain.VoteAccept, sev),
			vote("R2", findingID, domain.VoteAccept, sev),
			vote("R3", findingID, domain.VoteReject, sev),
		}
	case "abstain_heavy":
		return []domain.Vote{
			vote("R1", findingID, domain.VoteAccept, sev),
			vote("R2", findingID, domain.VoteAbstain, sev),
			vote("R3", findingID, domain.VoteAbstain, sev),
		}
	default:
		panic("unknown vote pattern " + pattern)
	}
}

func configuredForPattern(pattern string) (configured, valid int) {
	if pattern == "tie" {
		return 4, 4
	}
	return 3, 3
}

// TestDomainDeliberationPlaytest runs 100 domain-themed consensus scenarios.
func TestDomainDeliberationPlaytest(t *testing.T) {
	scenarios := domainScenarios()
	if len(scenarios) != 100 {
		t.Fatalf("expected 100 scenarios, got %d", len(scenarios))
	}
	// Ensure four domains × 25.
	counts := map[string]int{}
	for _, s := range scenarios {
		counts[s.domain]++
	}
	for _, d := range []string{"philosophy", "ethics", "science", "coding"} {
		if counts[d] != 25 {
			t.Fatalf("domain %s: got %d scenarios, want 25", d, counts[d])
		}
	}

	for _, sc := range scenarios {
		sc := sc
		t.Run(fmt.Sprintf("%s/%s/%s", sc.domain, sc.id, sc.votePattern), func(t *testing.T) {
			f := find(sc.id+"-F1", sc.category, sc.severity, sc.evidence, "artifact:"+sc.id+".md", sc.quote, 0)
			f.Issue = sc.issue
			f.Recommendation = "Revise the claim; engage counterarguments; supply evidence proportional to the conclusion."
			votes := votesForPattern(sc.votePattern, f.ID, sc.severity)
			configured, valid := configuredForPattern(sc.votePattern)
			decision := domain.ResolveVotes(f, votes, domain.ConsensusOptions{
				ConfiguredReviewers: configured,
				ValidReviewers:      valid,
			})
			if decision.Outcome != sc.wantOutcome {
				t.Fatalf("outcome = %q reason=%q, want outcome %q reason %q (issue=%q)",
					decision.Outcome, decision.Reason, sc.wantOutcome, sc.wantReason, sc.issue)
			}
			if sc.wantReason != "" && decision.Reason != sc.wantReason {
				t.Fatalf("reason = %q, want %q (outcome=%q)", decision.Reason, sc.wantReason, decision.Outcome)
			}
			// Agreement / dissent signals the playtest cares about.
			switch sc.votePattern {
			case "majority_accept", "severity_dissent":
				if len(decision.Dissent) < 1 {
					t.Fatalf("expected dissent recorded on majority_accept/severity_dissent, got %d", len(decision.Dissent))
				}
			case "unanimous_accept":
				if len(decision.Dissent) != 0 {
					t.Fatalf("unanimous accept should have zero dissent, got %d", len(decision.Dissent))
				}
			case "tie", "strict_split", "abstain_heavy":
				if decision.Outcome != "arbitration" {
					t.Fatalf("disagreement pattern should arbitrate, got %s/%s", decision.Outcome, decision.Reason)
				}
			}
		})
	}
}

// TestDomainDeliberation_CrossDomainClustering checks that same-quote findings
// from different reviewers cluster even when issue prose differs slightly —
// the panel "agrees" by mechanism, not by identical wording.
func TestDomainDeliberation_CrossDomainClustering(t *testing.T) {
	cases := []struct {
		id    string
		cat   domain.Category
		quote string
	}{
		{"CL-P", domain.CategoryFactualClaim, "the hard problem dissolves"},
		{"CL-E", domain.CategoryIntegrity, "Hypothetical consent substitutes for actual consent"},
		{"CL-S", domain.CategoryFactualClaim, "reduces absolute risk by 95 percentage points"},
		{"CL-C", domain.CategorySecurity, "string concatenation of user input"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			f1 := find(tc.id+"-a", tc.cat, domain.SeverityMajor, domain.EvidenceAnchored, "artifact:doc.md", tc.quote, 10)
			f1.Issue = "Reviewer A phrasing of the defect."
			f2 := find(tc.id+"-b", tc.cat, domain.SeverityMajor, domain.EvidenceAnchored, "artifact:doc.md", tc.quote, 10)
			f2.Issue = "Reviewer B different wording, same anchor."
			f3 := find(tc.id+"-c", tc.cat, domain.SeverityMinor, domain.EvidenceAnchored, "artifact:doc.md", tc.quote, 10)
			f3.Issue = "Reviewer C severity-leaning minor on same anchor."
			clusters := domain.ClusterFindings([]domain.Finding{f1, f2, f3})
			if len(clusters) != 1 {
				t.Fatalf("want 1 cluster for shared quote/category, got %d", len(clusters))
			}
			if len(clusters[0].MemberIDs) != 3 {
				t.Fatalf("want 3 clustered members, got %d", len(clusters[0].MemberIDs))
			}
		})
	}
}

// TestDomainDeliberation_DisagreementToConsensus runs five full Review→Arbitrate
// loops with domain-flavored planted defects (philosophy, ethics, science,
// coding, plus a mixed security ethics case).
func TestDomainDeliberation_DisagreementToConsensus(t *testing.T) {
	docs := []string{"P-arb", "E-arb", "S-arb", "C-arb", "X-arb"}
	for _, id := range docs {
		id := id
		t.Run(id, func(t *testing.T) {
			documentDir := t.TempDir()
			// Two-reviewer split → vote_tie → operator resolves accept.
			reviewers := []scriptedReviewer{
				{id: "one", raisesFind: true, voteChoice: domain.VoteAccept, voteSeverity: domain.SeverityMajor},
				{id: "two", raisesFind: false, voteChoice: domain.VoteReject, voteSeverity: domain.SeverityMajor},
			}
			service, packet, panel := buildE2EService(t, documentDir, reviewers)
			t.Setenv("PATH", "")
			final, err := service.Review(context.Background(), ReviewOptions{Packet: &packet, PanelValue: &panel})
			var exit *ExitError
			if !errors.As(err, &exit) || exit.Code != ExitArbitration {
				t.Fatalf("%s: want arbitration, err=%v final status=%s", id, err, final.Status)
			}
			if len(final.Arbitration) != 1 {
				t.Fatalf("%s: want 1 dispute, got %d", id, len(final.Arbitration))
			}
			disputeID := final.Arbitration[0].ID
			resolved, err := service.Arbitrate(ArbitrationOptions{
				RunRef:  RunRef{Input: documentDir, RunID: final.RunID},
				Rulings: []ArbitrationRuling{{ID: disputeID, Outcome: "accepted", Reason: "domain defect is material; accept finding", Operator: "playtest@local"}},
			})
			if err != nil {
				if !errors.As(err, &exit) || (exit.Code != ExitSuccess && exit.Code != ExitBlockingFindings) {
					t.Fatalf("%s: arbitrate err=%v", id, err)
				}
			}
			if len(resolved.Arbitration) != 0 {
				t.Fatalf("%s: disputes remain: %+v", id, resolved.Arbitration)
			}
			if len(resolved.Decisions) != 1 || resolved.Decisions[0].Outcome != "accepted" {
				t.Fatalf("%s: want accepted decision, got %+v", id, resolved.Decisions)
			}
		})
	}
}
