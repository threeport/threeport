package v0

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	// EksTokenPrefix is prepended to the base64-encoded presigned STS URL that
	// forms an EKS bearer token.
	EksTokenPrefix = "k8s-aws-v1."

	// EksClusterIdHeader is the STS request header that binds a token to a
	// single EKS cluster.  It must be part of the request signature or the
	// cluster's API server will reject the token.
	EksClusterIdHeader = "x-k8s-aws-id"

	// eksPresignExpiresParam is the query parameter that carries the lifetime
	// of a presigned request.  The AWS SDK does not set it, and the
	// authenticator rejects a token whose presigned URL is missing it.
	eksPresignExpiresParam = "X-Amz-Expires"

	// eksPresignExpiresSeconds is the lifetime we ask for when presigning.
	// The authenticator only accepts a value greater than zero and no more
	// than 900 seconds.
	eksPresignExpiresSeconds = "60"

	// EksTokenExpiration is how long we consider an EKS token good for.  AWS
	// expires these tokens 15 minutes after the request is signed no matter
	// what expiry was asked for when presigning, so we report a minute less to
	// avoid handing out a token that is about to go stale.
	EksTokenExpiration = 14 * time.Minute
)

// EksClusterConnectionInfo contains the information needed to connect to an EKS
// cluster.
type EksClusterConnectionInfo struct {
	ClusterName     string
	APIEndpoint     string
	CACertificate   string
	Token           string
	TokenExpiration time.Time
}

// Get retrieves connection info for a given EKS cluster by name.
func (c *EksClusterConnectionInfo) Get(awsConfig *aws.Config) error {
	ctx := context.Background()

	// get the cluster's API endpoint and certificate authority
	eksClient := eks.NewFromConfig(*awsConfig)
	describeClusterOutput, err := eksClient.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(c.ClusterName),
	})
	if err != nil {
		return fmt.Errorf("failed to describe EKS cluster: %w", err)
	}
	cluster := describeClusterOutput.Cluster
	if cluster == nil || cluster.Endpoint == nil ||
		cluster.CertificateAuthority == nil || cluster.CertificateAuthority.Data == nil {
		return fmt.Errorf("EKS cluster %s returned incomplete connection info", c.ClusterName)
	}
	caCertificate, err := base64.StdEncoding.DecodeString(*cluster.CertificateAuthority.Data)
	if err != nil {
		return fmt.Errorf("failed to decode CA data: %w", err)
	}

	// get a bearer token to authenticate to the cluster's Kubernetes API
	token, tokenExpiration, err := GetEksToken(ctx, awsConfig, c.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to get EKS cluster token: %w", err)
	}

	c.APIEndpoint = *cluster.Endpoint
	c.CACertificate = string(caCertificate)
	c.Token = token
	c.TokenExpiration = tokenExpiration

	return nil
}

// GetEksToken returns a bearer token that authenticates the caller to an EKS
// cluster's Kubernetes API along with the time at which it expires.  The token
// is a presigned STS GetCallerIdentity URL that carries the cluster name as a
// signed header - the format the AWS IAM authenticator running in the EKS
// control plane expects.
func GetEksToken(
	ctx context.Context,
	awsConfig *aws.Config,
	clusterName string,
) (string, time.Time, error) {
	presignClient := sts.NewPresignClient(sts.NewFromConfig(*awsConfig))
	presignedRequest, err := presignClient.PresignGetCallerIdentity(
		ctx,
		&sts.GetCallerIdentityInput{},
		withEksPresignCustomizations(clusterName),
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to presign STS get caller identity request: %w", err)
	}

	token := EksTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(presignedRequest.URL))

	return token, time.Now().UTC().Add(EksTokenExpiration), nil
}

// withEksPresignCustomizations returns a presign option that prepares the STS
// request for use as an EKS token before it is signed.
func withEksPresignCustomizations(clusterName string) func(*sts.PresignOptions) {
	return func(presignOptions *sts.PresignOptions) {
		presignOptions.ClientOptions = append(
			presignOptions.ClientOptions,
			func(stsOptions *sts.Options) {
				stsOptions.APIOptions = append(
					stsOptions.APIOptions,
					func(stack *middleware.Stack) error {
						return stack.Build.Add(
							eksPresignMiddleware{clusterName: clusterName},
							middleware.After,
						)
					},
				)
			},
		)
	}
}

// eksPresignMiddleware adds the cluster ID header and the presigned request
// lifetime to an STS request while it is being built, which is before it gets
// signed.  Both end up covered by the request signature.
type eksPresignMiddleware struct {
	clusterName string
}

// ID returns the unique identifier for this middleware.
func (m eksPresignMiddleware) ID() string {
	return "ThreeportEksPresign"
}

// HandleBuild adds the EKS cluster ID header and the expiry query parameter to
// the outgoing request.
func (m eksPresignMiddleware) HandleBuild(
	ctx context.Context,
	in middleware.BuildInput,
	next middleware.BuildHandler,
) (middleware.BuildOutput, middleware.Metadata, error) {
	request, ok := in.Request.(*smithyhttp.Request)
	if !ok {
		return middleware.BuildOutput{}, middleware.Metadata{}, fmt.Errorf(
			"unexpected request type %T when preparing EKS token request",
			in.Request,
		)
	}
	request.Header.Set(EksClusterIdHeader, m.clusterName)

	// the SDK leaves the expiry of a presigned request to the caller
	query := request.URL.Query()
	query.Set(eksPresignExpiresParam, eksPresignExpiresSeconds)
	request.URL.RawQuery = query.Encode()

	return next.HandleBuild(ctx, in)
}
