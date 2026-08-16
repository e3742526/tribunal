package domain

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Panel selection is a host-side pure function by design. No model chooses
// the panel, ranks its peers, or resolves a seat: the host reads a declared
// policy plus a declared catalog and computes a deterministic assignment.
// Handing this decision to a model would make the independence barrier
// depend on trusting one panelist to compose the others.

// Selection search bounds. The resolver is exhaustive within these bounds and
// reports when it truncated, so a large catalog degrades into a documented
// best-of-branch result rather than an unbounded search or a silent cut.
const (
	selectionMaxBranch = 8
	selectionMaxNodes  = 50000
	selectionMaxSeats  = 8
)

var selectionSlug = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// ModelCandidate is one selectable reviewer seat occupant. Quality,
// Reliability, and Cost are operator-declared priors, not measurements
// Tribunal makes: the resolver never infers them from a model name.
type ModelCandidate struct {
	ID                   string   `json:"id" toml:"id"`
	Adapter              string   `json:"adapter" toml:"adapter"`
	Model                string   `json:"model" toml:"model"`
	Family               string   `json:"family" toml:"family"`
	Capabilities         []string `json:"capabilities,omitempty" toml:"capabilities"`
	Quality              float64  `json:"quality" toml:"quality"`
	Reliability          float64  `json:"reliability" toml:"reliability"`
	Cost                 float64  `json:"cost" toml:"cost"`
	Weight               float64  `json:"weight" toml:"weight"`
	MaxContextTokens     int      `json:"max_context_tokens" toml:"max_context_tokens"`
	ReservedOutputTokens int      `json:"reserved_output_tokens" toml:"reserved_output_tokens"`
}

// PanelRole is one declared seat. Require is a hard filter; Prefer is a
// ranked soft signal. Persona names the bounded lens the seat carries, which
// is how a policy expresses "this seat is the foundation reviewer" without
// the resolver knowing anything about review semantics.
type PanelRole struct {
	Name     string   `json:"name" toml:"name"`
	Persona  string   `json:"persona,omitempty" toml:"persona"`
	Require  []string `json:"require,omitempty" toml:"require"`
	Prefer   []string `json:"prefer,omitempty" toml:"prefer"`
	Optional bool     `json:"optional,omitempty" toml:"optional"`
}

// PanelPolicy declares what a panel must satisfy, not which models to use.
// IndependentFamilies is the epistemic constraint the whole mechanism exists
// for: three samples from one family are correlated, not independent.
type PanelPolicy struct {
	SchemaVersion       int         `json:"schema_version" toml:"schema_version"`
	Name                string      `json:"name" toml:"name"`
	Summary             string      `json:"summary,omitempty" toml:"summary"`
	Roles               []PanelRole `json:"roles" toml:"roles"`
	MinimumPanel        int         `json:"minimum_panel" toml:"minimum_panel"`
	IndependentFamilies int         `json:"independent_families" toml:"independent_families"`
	DiversityWeight     float64     `json:"diversity_weight" toml:"diversity_weight"`
	ReliabilityWeight   float64     `json:"reliability_weight" toml:"reliability_weight"`
	CostWeight          float64     `json:"cost_weight" toml:"cost_weight"`
}

// SeatAssignment records why one seat resolved the way it did. Rationale is
// display text; every field it summarizes is present separately so callers
// never parse it.
type SeatAssignment struct {
	ReviewerID  string  `json:"reviewer_id"`
	Role        string  `json:"role"`
	CandidateID string  `json:"candidate_id"`
	Persona     string  `json:"persona"`
	Family      string  `json:"family"`
	Fit         float64 `json:"fit"`
	Score       float64 `json:"score"`
	Rationale   string  `json:"rationale"`
}

type PanelSelection struct {
	SchemaVersion int              `json:"schema_version"`
	Policy        string           `json:"policy"`
	Panel         Panel            `json:"panel"`
	Seats         []SeatAssignment `json:"seats"`
	Families      []string         `json:"families"`
	Utility       float64          `json:"utility"`
	DiversityNote string           `json:"diversity_note"`
	// Notes records every way the resolved panel fell short of the declared
	// policy or the search was bounded. An empty Notes means the policy was
	// satisfied exactly; it is never used to hide a dropped seat.
	Notes []string `json:"notes,omitempty"`
}

func ValidateCandidate(c ModelCandidate) error {
	if !selectionSlug.MatchString(c.ID) {
		return fmt.Errorf("model candidate id %q must match [a-z0-9-]{1,64}", c.ID)
	}
	if strings.TrimSpace(c.Adapter) == "" || strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model candidate %q requires adapter and model", c.ID)
	}
	if strings.ContainsRune(c.Adapter, '/') {
		return fmt.Errorf("model candidate %q adapter must not contain /", c.ID)
	}
	for _, value := range []float64{c.Quality, c.Reliability, c.Cost, c.Weight} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("model candidate %q has a non-finite score", c.ID)
		}
	}
	if c.Quality < 0 || c.Quality > 1 || c.Reliability < 0 || c.Reliability > 1 {
		return fmt.Errorf("model candidate %q quality and reliability must be within 0..1", c.ID)
	}
	if c.Cost < 0 {
		return fmt.Errorf("model candidate %q cost must not be negative", c.ID)
	}
	for _, tag := range c.Capabilities {
		if !selectionSlug.MatchString(tag) {
			return fmt.Errorf("model candidate %q capability %q must match [a-z0-9-]{1,64}", c.ID, tag)
		}
	}
	return nil
}

func ValidatePolicy(p PanelPolicy) error {
	if p.SchemaVersion != SchemaVersion {
		return fmt.Errorf("panel policy schema_version must be %d", SchemaVersion)
	}
	if !selectionSlug.MatchString(p.Name) {
		return fmt.Errorf("panel policy name %q must match [a-z0-9-]{1,64}", p.Name)
	}
	if len(p.Roles) == 0 {
		return fmt.Errorf("panel policy %q requires at least one role", p.Name)
	}
	if len(p.Roles) > selectionMaxSeats {
		return fmt.Errorf("panel policy %q declares %d roles; the maximum is %d", p.Name, len(p.Roles), selectionMaxSeats)
	}
	seen := map[string]struct{}{}
	required := 0
	for _, role := range p.Roles {
		if !selectionSlug.MatchString(role.Name) {
			return fmt.Errorf("panel policy %q role name %q must match [a-z0-9-]{1,64}", p.Name, role.Name)
		}
		if _, exists := seen[role.Name]; exists {
			return fmt.Errorf("panel policy %q has duplicate role %q", p.Name, role.Name)
		}
		seen[role.Name] = struct{}{}
		if role.Persona != "" && !selectionSlug.MatchString(role.Persona) {
			return fmt.Errorf("panel policy %q role %q has invalid persona", p.Name, role.Name)
		}
		for _, tag := range append(append([]string{}, role.Require...), role.Prefer...) {
			if !selectionSlug.MatchString(tag) {
				return fmt.Errorf("panel policy %q role %q has invalid capability tag %q", p.Name, role.Name, tag)
			}
		}
		if !role.Optional {
			required++
		}
	}
	// A quorum below two cannot produce consensus, so a policy that declares
	// one is asking for a panel Tribunal would immediately report degraded.
	if p.MinimumPanel < 2 {
		return fmt.Errorf("panel policy %q minimum_panel must be at least 2", p.Name)
	}
	if p.MinimumPanel > len(p.Roles) {
		return fmt.Errorf("panel policy %q minimum_panel %d exceeds its %d declared roles", p.Name, p.MinimumPanel, len(p.Roles))
	}
	if required > p.MinimumPanel {
		return fmt.Errorf("panel policy %q requires %d non-optional roles but declares minimum_panel %d", p.Name, required, p.MinimumPanel)
	}
	if p.IndependentFamilies < 1 {
		return fmt.Errorf("panel policy %q independent_families must be at least 1", p.Name)
	}
	if p.IndependentFamilies > p.MinimumPanel {
		return fmt.Errorf("panel policy %q independent_families %d exceeds minimum_panel %d", p.Name, p.IndependentFamilies, p.MinimumPanel)
	}
	for name, value := range map[string]float64{"diversity_weight": p.DiversityWeight, "reliability_weight": p.ReliabilityWeight, "cost_weight": p.CostWeight} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return fmt.Errorf("panel policy %q %s must be a finite non-negative number", p.Name, name)
		}
	}
	return nil
}

// capabilityFit scores how well a candidate matches a role's ranked Prefer
// list. Earlier entries carry more weight; an empty Prefer list is a neutral
// 0 so a policy that states no preference does not silently favor whichever
// candidate happens to carry the most unrelated tags.
func capabilityFit(prefer []string, capabilities []string) float64 {
	if len(prefer) == 0 {
		return 0
	}
	has := make(map[string]struct{}, len(capabilities))
	for _, tag := range capabilities {
		has[tag] = struct{}{}
	}
	var matched, total float64
	for index, tag := range prefer {
		weight := float64(len(prefer) - index)
		total += weight
		if _, ok := has[tag]; ok {
			matched += weight
		}
	}
	if total == 0 {
		return 0
	}
	return matched / total
}

func satisfiesRequire(require []string, capabilities []string) bool {
	if len(require) == 0 {
		return true
	}
	has := make(map[string]struct{}, len(capabilities))
	for _, tag := range capabilities {
		has[tag] = struct{}{}
	}
	for _, tag := range require {
		if _, ok := has[tag]; !ok {
			return false
		}
	}
	return true
}

// seatScore is the family-independent part of a candidate's contribution:
// quality blended with role fit, plus reliability, minus normalized cost.
// The diversity term is deliberately excluded here because it depends on the
// rest of the panel and is added once per distinct family in the objective.
func seatScore(policy PanelPolicy, role PanelRole, c ModelCandidate, maxCost float64) (score, fit float64) {
	fit = capabilityFit(role.Prefer, c.Capabilities)
	quality := 0.5*c.Quality + 0.5*fit
	cost := 0.0
	if maxCost > 0 {
		cost = c.Cost / maxCost
	}
	return quality + policy.ReliabilityWeight*c.Reliability - policy.CostWeight*cost, fit
}

type seatOption struct {
	candidate ModelCandidate
	score     float64
	fit       float64
}

type searchState struct {
	best      []int
	bestScore float64
	bestFound bool
	nodes     int
	truncated bool
}

// SelectPanel resolves a policy against a catalog into a concrete panel.
//
// The objective is U(P) = Σ(seat score) + diversity_weight × |distinct
// families|, maximized subject to hard constraints: every non-optional role
// filled, at least minimum_panel seats, at least independent_families
// distinct families, and no candidate seated twice. Infeasibility is an
// error, never a quietly smaller panel.
//
// Context budgets stay at whatever the catalog declared, including zero. The
// caller applies its configured limits and then calls NormalizePanel — the
// same order the string-panel path uses — so a catalog entry cannot raise its
// own context ceiling past the configured one.
func SelectPanel(policy PanelPolicy, catalog []ModelCandidate) (PanelSelection, error) {
	if err := ValidatePolicy(policy); err != nil {
		return PanelSelection{}, err
	}
	seen := map[string]struct{}{}
	for _, candidate := range catalog {
		if err := ValidateCandidate(candidate); err != nil {
			return PanelSelection{}, err
		}
		if _, exists := seen[candidate.ID]; exists {
			return PanelSelection{}, fmt.Errorf("duplicate model candidate id %q", candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
	}
	if len(catalog) < policy.MinimumPanel {
		return PanelSelection{}, fmt.Errorf("panel policy %q needs %d reviewers but the catalog offers %d", policy.Name, policy.MinimumPanel, len(catalog))
	}
	normalized := normalizeCatalog(catalog)
	maxCost := 0.0
	for _, candidate := range normalized {
		if candidate.Cost > maxCost {
			maxCost = candidate.Cost
		}
	}
	options := make([][]seatOption, len(policy.Roles))
	var truncatedRoles []string
	for i, role := range policy.Roles {
		for _, candidate := range normalized {
			if !satisfiesRequire(role.Require, candidate.Capabilities) {
				continue
			}
			score, fit := seatScore(policy, role, candidate, maxCost)
			options[i] = append(options[i], seatOption{candidate: candidate, score: score, fit: fit})
		}
		sort.SliceStable(options[i], func(a, b int) bool {
			if options[i][a].score != options[i][b].score {
				return options[i][a].score > options[i][b].score
			}
			return options[i][a].candidate.ID < options[i][b].candidate.ID
		})
		if len(options[i]) > selectionMaxBranch {
			options[i] = options[i][:selectionMaxBranch]
			truncatedRoles = append(truncatedRoles, role.Name)
		}
		if len(options[i]) == 0 && !role.Optional {
			return PanelSelection{}, fmt.Errorf("panel policy %q role %q requires capabilities no catalog model declares", policy.Name, role.Name)
		}
	}
	state := &searchState{}
	assignment := make([]int, len(policy.Roles))
	for i := range assignment {
		assignment[i] = -1
	}
	search(policy, options, assignment, 0, map[string]struct{}{}, map[string]struct{}{}, 0, state)
	if !state.bestFound {
		return PanelSelection{}, fmt.Errorf("panel policy %q cannot be satisfied: no assignment reaches %d reviewers across %d independent families", policy.Name, policy.MinimumPanel, policy.IndependentFamilies)
	}
	selection := buildSelection(policy, options, state)
	for _, role := range truncatedRoles {
		selection.Notes = append(selection.Notes, fmt.Sprintf("role %s considered only the %d highest-scoring candidates", role, selectionMaxBranch))
	}
	if state.truncated {
		selection.Notes = append(selection.Notes, fmt.Sprintf("selection search stopped at the %d-node bound; the result is the best assignment found within it", selectionMaxNodes))
	}
	return selection, nil
}

// normalizeCatalog fills defaults that make an entry usable without asserting
// anything about the model: an absent family falls back to the adapter, which
// is the coarsest honest independence unit available.
func normalizeCatalog(catalog []ModelCandidate) []ModelCandidate {
	normalized := make([]ModelCandidate, 0, len(catalog))
	for _, candidate := range catalog {
		if strings.TrimSpace(candidate.Family) == "" {
			candidate.Family = candidate.Adapter
		}
		if candidate.Weight == 0 {
			candidate.Weight = 1
		}
		tags := append([]string(nil), candidate.Capabilities...)
		sort.Strings(tags)
		candidate.Capabilities = tags
		normalized = append(normalized, candidate)
	}
	sort.SliceStable(normalized, func(i, j int) bool { return normalized[i].ID < normalized[j].ID })
	return normalized
}

func search(policy PanelPolicy, options [][]seatOption, assignment []int, seat int, usedIDs, families map[string]struct{}, score float64, state *searchState) {
	if state.nodes >= selectionMaxNodes {
		state.truncated = true
		return
	}
	state.nodes++
	if seat == len(policy.Roles) {
		filled := 0
		for _, choice := range assignment {
			if choice >= 0 {
				filled++
			}
		}
		if filled < policy.MinimumPanel || len(families) < policy.IndependentFamilies {
			return
		}
		total := score + policy.DiversityWeight*float64(len(families))
		if !state.bestFound || total > state.bestScore {
			state.bestFound = true
			state.bestScore = total
			state.best = append([]int(nil), assignment...)
		}
		return
	}
	for index, option := range options[seat] {
		if _, taken := usedIDs[option.candidate.ID]; taken {
			continue
		}
		usedIDs[option.candidate.ID] = struct{}{}
		_, familySeen := families[option.candidate.Family]
		if !familySeen {
			families[option.candidate.Family] = struct{}{}
		}
		assignment[seat] = index
		search(policy, options, assignment, seat+1, usedIDs, families, score+option.score, state)
		assignment[seat] = -1
		if !familySeen {
			delete(families, option.candidate.Family)
		}
		delete(usedIDs, option.candidate.ID)
	}
	if policy.Roles[seat].Optional {
		search(policy, options, assignment, seat+1, usedIDs, families, score, state)
	}
}

func buildSelection(policy PanelPolicy, options [][]seatOption, state *searchState) PanelSelection {
	selection := PanelSelection{SchemaVersion: SchemaVersion, Policy: policy.Name, Utility: state.bestScore}
	panel := Panel{SchemaVersion: SchemaVersion}
	familySet := map[string]struct{}{}
	for seat, choice := range state.best {
		role := policy.Roles[seat]
		if choice < 0 {
			selection.Notes = append(selection.Notes, fmt.Sprintf("optional role %s left unfilled", role.Name))
			continue
		}
		option := options[seat][choice]
		persona := role.Persona
		if persona == "" {
			persona = "plain"
		}
		id := fmt.Sprintf("R-%03d", len(panel.Reviewers)+1)
		panel.Reviewers = append(panel.Reviewers, Panelist{
			ID:                   id,
			Adapter:              option.candidate.Adapter,
			Model:                option.candidate.Model,
			Family:               option.candidate.Family,
			Persona:              persona,
			Weight:               option.candidate.Weight,
			MaxContextTokens:     option.candidate.MaxContextTokens,
			ReservedOutputTokens: option.candidate.ReservedOutputTokens,
		})
		familySet[option.candidate.Family] = struct{}{}
		selection.Seats = append(selection.Seats, SeatAssignment{
			ReviewerID:  id,
			Role:        role.Name,
			CandidateID: option.candidate.ID,
			Persona:     persona,
			Family:      option.candidate.Family,
			Fit:         option.fit,
			Score:       option.score,
			Rationale: fmt.Sprintf("role %s: %s/%s from family %s (fit %.2f, quality %.2f, reliability %.2f, cost %.2f)",
				role.Name, option.candidate.Adapter, option.candidate.Model, option.candidate.Family,
				option.fit, option.candidate.Quality, option.candidate.Reliability, option.candidate.Cost),
		})
	}
	families := make([]string, 0, len(familySet))
	for family := range familySet {
		families = append(families, family)
	}
	sort.Strings(families)
	selection.Families = families
	selection.Panel = panel
	selection.DiversityNote = DiversityNote(panel)
	if len(families) < policy.IndependentFamilies {
		selection.Notes = append(selection.Notes, fmt.Sprintf("resolved %d independent families; policy asked for %d", len(families), policy.IndependentFamilies))
	}
	return selection
}
