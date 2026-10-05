package v0

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
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

	// eksPresignExpiresSeconds is the lifetime asked for when presigning, and
	// it does not bound how long the token is good for.
	//
	// STS ignores this value on a presigned GetCallerIdentity URL: the request
	// is valid for fifteen minutes from the x-amz-date it was signed at, no
	// matter what is asked for here.  The value exists because the
	// authenticator in the EKS control plane rejects a URL whose x-amz-expires
	// is missing or outside 0 to 900 seconds, and because authenticators at
	// 0.3.0 and older required one in that range.  Sixty seconds is what
	// sigs.k8s.io/aws-iam-authenticator has always sent, so it is what we send.
	//
	// This is why EksTokenExpiration below is fourteen minutes rather than one:
	// the two numbers describe different things, not the same thing twice.
	eksPresignExpiresSeconds = "60"

	// EksTokenExpiration is how long an EKS token is good for: fifteen minutes
	// from signing, less a minute so that a token handed to a caller is not
	// about to go stale in their hands.  See eksPresignExpiresSeconds above
	// for why the presigned URL asks for sixty seconds and this is still
	// correct.
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
	// a caller that failed to resolve a config reaches here with nil, and a
	// nil dereference buries the real problem under a stack trace
	if awsConfig == nil {
		return errors.New("cannot get EKS cluster connection info without an AWS config")
	}

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

// EksOidcIssuerUrl returns the URL of the OIDC issuer EKS stands up with a
// cluster.  The issuer is registered as an IAM identity provider so that the
// cluster's service accounts can assume IAM roles, and its URL is part of the
// trust policy of every role bound that way.  It carries an ID assigned at
// cluster creation, so it can only be read from AWS.
func EksOidcIssuerUrl(awsConfig *aws.Config, clusterName string) (string, error) {
	if awsConfig == nil {
		return "", errors.New("cannot get the EKS OIDC issuer without an AWS config")
	}

	eksClient := eks.NewFromConfig(*awsConfig)
	describeClusterOutput, err := eksClient.DescribeCluster(
		context.Background(),
		&eks.DescribeClusterInput{Name: aws.String(clusterName)},
	)
	if err != nil {
		return "", fmt.Errorf("failed to describe EKS cluster: %w", err)
	}
	cluster := describeClusterOutput.Cluster
	if cluster == nil || cluster.Identity == nil || cluster.Identity.Oidc == nil ||
		cluster.Identity.Oidc.Issuer == nil {
		return "", fmt.Errorf("EKS cluster %s reports no OIDC issuer", clusterName)
	}

	return *cluster.Identity.Oidc.Issuer, nil
}

// EksStorageAddonActive reports whether the EBS CSI addon is installed and
// running on a cluster.  It is the last thing the EKS create does, so it
// stands for the create having finished rather than merely started.
//
// A cluster that does not exist, or an addon that was never created, reads as
// not complete rather than as an error: both are ordinary states partway
// through a create.
func EksStorageAddonActive(awsConfig *aws.Config, clusterName string) (bool, error) {
	if awsConfig == nil {
		return false, errors.New("cannot check the EKS storage addon without an AWS config")
	}

	describeAddonOutput, err := eks.NewFromConfig(*awsConfig).DescribeAddon(
		context.Background(),
		&eks.DescribeAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String(EksEbsStorageAddonName),
		},
	)
	if err != nil {
		var notFound *ekstypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe the %s addon: %w", EksEbsStorageAddonName, err)
	}
	if describeAddonOutput.Addon == nil {
		return false, nil
	}

	return describeAddonOutput.Addon.Status == ekstypes.AddonStatusActive, nil
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
