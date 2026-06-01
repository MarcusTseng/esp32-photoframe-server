package publicart

import "sort"

func RankCandidates(candidates []Candidate, cfg Config) []Candidate {
	ranked := append([]Candidate(nil), candidates...)
	sort.SliceStable(ranked, func(i, j int) bool {
		si := candidateScore(ranked[i], cfg)
		sj := candidateScore(ranked[j], cfg)
		if si != sj {
			return si > sj
		}
		return ranked[i].Title < ranked[j].Title
	})
	return ranked
}

func candidateScore(c Candidate, cfg Config) int {
	longEdge := c.Width
	if c.Height > longEdge {
		longEdge = c.Height
	}
	score := 0
	if c.ImageURL != "" {
		score += 100
	}
	if cfg.PreferredImageLongEdge > 0 && longEdge >= cfg.PreferredImageLongEdge {
		score += 50
	} else if cfg.MinImageLongEdge > 0 && longEdge >= cfg.MinImageLongEdge {
		score += 25
	}
	if candidateMatchesOrientation(c, cfg.Orientation) {
		score += 75
	}
	return score
}

func candidateMatchesOrientation(c Candidate, orientation string) bool {
	if orientation == "" || c.Width <= 0 || c.Height <= 0 || c.Width == c.Height {
		return false
	}
	if orientation == "landscape" {
		return c.Width > c.Height
	}
	if orientation == "portrait" {
		return c.Height > c.Width
	}
	return false
}
