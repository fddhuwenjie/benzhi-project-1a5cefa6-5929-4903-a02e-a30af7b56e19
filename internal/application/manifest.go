package application

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"sort"
)

func evidenceManifest(c *domain.ExerciseCase) []domain.EvidenceRef {
	seen := map[string]domain.EvidenceRef{}
	for _, result := range c.Evaluations {
		if !result.Passed {
			continue
		}
		for _, source := range result.EvidenceSources {
			for _, ref := range source.EvidenceRefs {
				ref = domain.NormalizeEvidence(ref)
				seen[domain.EvidenceKey(ref)] = ref
			}
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	manifest := make([]domain.EvidenceRef, 0, len(keys))
	for _, key := range keys {
		manifest = append(manifest, seen[key])
	}
	return manifest
}
