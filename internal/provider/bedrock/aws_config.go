package bedrock

import (
	"context"
	"fmt"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// LoadAWSConfig builds aws.Config for Bedrock Runtime and control-plane clients using the same
// rules as the LLM provider (API key bearer, static keys, named profile, then default chain).
func LoadAWSConfig(ctx context.Context, cfg Config) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if cfg.Region != "" {
		opts = append(opts, awsconfig.WithRegion(cfg.Region))
	}
	if cfg.APIKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
		transport := http.DefaultTransport
		opts = append(opts, awsconfig.WithHTTPClient(&http.Client{
			Transport: &apiKeyTransport{
				token: cfg.APIKey,
				base:  transport,
			},
		}))
	} else if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	} else if cfg.Profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		if cfg.APIKey != "" {
			return aws.Config{
				Region:      cfg.Region,
				Credentials: aws.AnonymousCredentials{},
				HTTPClient: &http.Client{
					Transport: &apiKeyTransport{
						token: cfg.APIKey,
						base:  http.DefaultTransport,
					},
				},
			}, nil
		}
		if cfg.AccessKeyID != "" {
			return aws.Config{
				Region: cfg.Region,
				Credentials: aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
					return aws.Credentials{
						AccessKeyID:     cfg.AccessKeyID,
						SecretAccessKey: cfg.SecretAccessKey,
					}, nil
				}),
			}, nil
		}
		return aws.Config{}, fmt.Errorf("bedrock: load aws config: %w", err)
	}
	return awsCfg, nil
}
