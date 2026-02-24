package embedproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/assistclaw/assistclaw/internal/embeddings"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

type bedrockEmbedder struct {
	client *bedrockruntime.Client
	region string
}

func NewBedrock(region, profile, accessKey, secretKey, apiKey string) (embeddings.Embedder, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var opts []func(*config.LoadOptions) error
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if accessKey != "" && secretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, err
	}

	client := bedrockruntime.NewFromConfig(cfg)
	return &bedrockEmbedder{client: client, region: region}, nil
}

func (e *bedrockEmbedder) Name() string                        { return "bedrock" }
func (e *bedrockEmbedder) DefaultModel() string                { return "amazon.titan-embed-text-v2:0" }
func (e *bedrockEmbedder) HealthCheck(_ context.Context) error { return nil }

func (e *bedrockEmbedder) ListModels(_ context.Context) ([]embeddings.ModelInfo, error) {
	return []embeddings.ModelInfo{
		{ID: "amazon.titan-embed-text-v2:0", Name: "Titan Text Embed v2", Provider: "bedrock", Dimensions: 1024},
		{ID: "amazon.titan-embed-text-v1", Name: "Titan Text Embed v1", Provider: "bedrock", Dimensions: 1536},
		{ID: "cohere.embed-english-v3", Name: "Cohere Embed English v3", Provider: "bedrock", Dimensions: 1024},
		{ID: "cohere.embed-multilingual-v3", Name: "Cohere Embed Multilingual v3", Provider: "bedrock", Dimensions: 1024},
	}, nil
}

func (e *bedrockEmbedder) Embed(ctx context.Context, req *embeddings.EmbedRequest) (*embeddings.EmbedResponse, error) {
	model := req.Model
	if model == "" {
		model = e.DefaultModel()
	}

	// Bedrock requires individual calls or specific batch formats depending on model family.
	// Titan v2 uses a specific JSON format.
	var vecs [][]float32
	for _, text := range req.Texts {
		var body []byte
		var err error

		// Simple implementation for Titan
		input := map[string]any{"inputText": text}
		body, err = json.Marshal(input)
		if err != nil {
			return nil, err
		}

		output, err := e.client.InvokeModel(ctx, &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(model),
			ContentType: aws.String("application/json"),
			Body:        body,
		})
		if err != nil {
			return nil, err
		}

		var resp struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.NewDecoder(bytes.NewReader(output.Body)).Decode(&resp); err != nil {
			return nil, err
		}
		vecs = append(vecs, resp.Embedding)
	}

	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}

	return &embeddings.EmbedResponse{
		Embeddings: vecs,
		Dimensions: dim,
		Model:      model,
	}, nil
}
