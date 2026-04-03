package bedrock

import (
	"context"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// ModelPick is a single selectable Bedrock foundation model for onboarding UIs.
type ModelPick struct {
	ID    string
	Label string
}

// ListOnboardingTextModels lists ON_DEMAND foundation models in cfg.Region that output TEXT
// (typical chat/completion). Merges in ChatModelCatalog entries missing from the API response.
// Returns a sorted list by label. On total failure, returns the static catalog only.
func ListOnboardingTextModels(ctx context.Context, cfg Config) []ModelPick {
	byID := make(map[string]ModelPick)

	add := func(id, label string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := byID[id]; ok {
			return
		}
		if strings.TrimSpace(label) == "" {
			label = id
		}
		byID[id] = ModelPick{ID: id, Label: label}
	}

	awsCfg, err := LoadAWSConfig(ctx, cfg)
	if err == nil {
		c := bedrock.NewFromConfig(awsCfg)
		out, err := c.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
			ByInferenceType:  brtypes.InferenceTypeOnDemand,
			ByOutputModality: brtypes.ModelModalityText,
		})
		if err == nil && out != nil {
			for _, m := range out.ModelSummaries {
				if m.ModelId == nil || *m.ModelId == "" {
					continue
				}
				if m.ModelLifecycle != nil && m.ModelLifecycle.Status == brtypes.FoundationModelLifecycleStatusLegacy {
					continue
				}
				id := *m.ModelId
				label := id
				if m.ModelName != nil && strings.TrimSpace(*m.ModelName) != "" {
					label = strings.TrimSpace(*m.ModelName) + " — " + id
				}
				if m.ProviderName != nil && strings.TrimSpace(*m.ProviderName) != "" {
					label = strings.TrimSpace(*m.ProviderName) + " · " + label
				}
				add(id, label)
			}
		}
	}

	for _, mi := range bedrockModelCatalog(providerName) {
		add(mi.ID, mi.Name+" — "+mi.ID)
	}

	out := make([]ModelPick, 0, len(byID))
	for _, p := range byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}

// ListOnboardingEmbeddingModels lists ON_DEMAND models with EMBEDDING output for the region.
// Falls back to a small static list if the API call fails.
func ListOnboardingEmbeddingModels(ctx context.Context, cfg Config) []ModelPick {
	byID := make(map[string]ModelPick)
	add := func(id, label string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := byID[id]; ok {
			return
		}
		byID[id] = ModelPick{ID: id, Label: label}
	}

	staticEmb := []ModelPick{
		{ID: "amazon.titan-embed-text-v2:0", Label: "Amazon Titan Text Embeddings v2"},
		{ID: "amazon.titan-embed-text-v1", Label: "Amazon Titan Text Embeddings v1"},
		{ID: "cohere.embed-english-v3", Label: "Cohere Embed English v3"},
		{ID: "cohere.embed-multilingual-v3", Label: "Cohere Embed Multilingual v3"},
	}

	awsCfg, err := LoadAWSConfig(ctx, cfg)
	if err == nil {
		c := bedrock.NewFromConfig(awsCfg)
		out, err := c.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{
			ByInferenceType:  brtypes.InferenceTypeOnDemand,
			ByOutputModality: brtypes.ModelModalityEmbedding,
		})
		if err == nil && out != nil {
			for _, m := range out.ModelSummaries {
				if m.ModelId == nil || *m.ModelId == "" {
					continue
				}
				if m.ModelLifecycle != nil && m.ModelLifecycle.Status == brtypes.FoundationModelLifecycleStatusLegacy {
					continue
				}
				id := *m.ModelId
				label := id
				if m.ModelName != nil && strings.TrimSpace(*m.ModelName) != "" {
					label = strings.TrimSpace(*m.ModelName) + " — " + id
				}
				add(id, label)
			}
		}
	}
	for _, p := range staticEmb {
		add(p.ID, p.Label+" — "+p.ID)
	}
	out := make([]ModelPick, 0, len(byID))
	for _, p := range byID {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label)
	})
	return out
}
