package app

import (
	"github.com/e3742526/tribunal/internal/tribunal/config"
	"github.com/e3742526/tribunal/internal/tribunal/domain"
)

func (s *Service) resolvePanel(opts ReviewOptions) (domain.Panel, error) {
	panel, _, err := s.resolvePanelWithSelection(opts)
	return panel, err
}

// resolvePanelWithSelection returns the panel and, when the panel came from a
// declarative policy, the selection record that explains it. Precedence runs
// most-specific first: a replayed panel value, an explicit panel string, then
// a policy, then the configured panel string. A replay or resume carries its
// recorded panel verbatim so a catalog edit cannot silently repanel a frozen
// packet.
func (s *Service) resolvePanelWithSelection(opts ReviewOptions) (domain.Panel, *domain.PanelSelection, error) {
	if opts.PanelValue != nil {
		panel := *opts.PanelValue
		if err := domain.NormalizePanel(&panel); err != nil {
			return domain.Panel{}, nil, err
		}
		panel, err := s.hydratePanel(panel)
		return panel, nil, err
	}
	if opts.Panel == "" {
		policy := opts.PanelPolicy
		if policy == "" {
			policy = s.Config.PanelPolicy
		}
		if policy != "" {
			return s.selectPanel(policy)
		}
	}
	raw := opts.Panel
	if raw == "" {
		raw = s.Config.Panel
	}
	panel, err := domain.ParsePanel(raw)
	if err != nil {
		return domain.Panel{}, nil, err
	}
	panel, err = s.finishPanel(panel)
	return panel, nil, err
}

func (s *Service) selectPanel(name string) (domain.Panel, *domain.PanelSelection, error) {
	policy, err := config.ResolvePanelPolicy(s.Config, name)
	if err != nil {
		return domain.Panel{}, nil, err
	}
	catalog, notes, err := config.PanelCatalog(s.Config)
	if err != nil {
		return domain.Panel{}, nil, err
	}
	selection, err := domain.SelectPanel(policy, catalog)
	if err != nil {
		return domain.Panel{}, nil, err
	}
	selection.Notes = append(notes, selection.Notes...)
	panel, err := s.finishPanel(selection.Panel)
	if err != nil {
		return domain.Panel{}, nil, err
	}
	selection.Panel = panel
	return panel, &selection, nil
}

// finishPanel applies the configured context budget and then validates. The
// budget is applied host-side after selection so neither a panel string nor a
// catalog entry can raise its own context ceiling.
func (s *Service) finishPanel(panel domain.Panel) (domain.Panel, error) {
	for i := range panel.Reviewers {
		panel.Reviewers[i].MaxContextTokens = s.Config.Limits.MaxContextTokens
		panel.Reviewers[i].ReservedOutputTokens = s.Config.Limits.ReservedOutput
	}
	if err := domain.NormalizePanel(&panel); err != nil {
		return domain.Panel{}, err
	}
	return s.hydratePanel(panel)
}

// hydratePanel resolves each seat's persona into the bounded lens text the
// reviewer prompt carries. The lens is still delivered as untrusted material;
// resolving it here only guarantees the seat is a real, lintable persona.
func (s *Service) hydratePanel(panel domain.Panel) (domain.Panel, error) {
	for i := range panel.Reviewers {
		if panel.Reviewers[i].Persona != "" && panel.Reviewers[i].Persona != "plain" {
			persona, err := config.ResolvePersona(panel.Reviewers[i].Persona, s.Config.WorkspaceRoot, s.Config.TrustWorkspace)
			if err != nil {
				return domain.Panel{}, err
			}
			panel.Reviewers[i].PersonaLens = config.PersonaText(persona)
		}
	}
	return panel, nil
}
