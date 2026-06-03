package publicart

// FallbackProvider uses the primary provider when it returns at least one
// candidate, and falls back to the secondary provider only when the primary
// fails or returns no usable image candidates.
type FallbackProvider struct {
	primary   Provider
	secondary Provider
}

func NewFallbackProvider(primary, secondary Provider) *FallbackProvider {
	return &FallbackProvider{primary: primary, secondary: secondary}
}

func (p *FallbackProvider) Search(query string, opts SearchOptions) ([]Candidate, error) {
	if p == nil || p.primary == nil {
		if p != nil && p.secondary != nil {
			return p.secondary.Search(query, opts)
		}
		return nil, nil
	}
	primaryCandidates, primaryErr := p.primary.Search(query, opts)
	if primaryErr == nil && len(primaryCandidates) > 0 {
		return primaryCandidates, nil
	}
	if p.secondary == nil {
		return primaryCandidates, primaryErr
	}
	secondaryCandidates, secondaryErr := p.secondary.Search(query, opts)
	if secondaryErr != nil {
		if primaryErr != nil {
			return nil, primaryErr
		}
		return primaryCandidates, nil
	}
	return secondaryCandidates, nil
}
