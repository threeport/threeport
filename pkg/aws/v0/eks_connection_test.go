package v0

import (
	"context"
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/threeport/threeport/pkg/encryption/v0"
)

// testAwsConfig returns an AWS config with static credentials so that tokens
// can be generated without reaching out to AWS.
func testAwsConfig() *aws.Config {
	return &aws.Config{
		Region: "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(
			"AKIAIOSFODNN7EXAMPLE",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"",
		),
	}
}

// decodeEksToken strips the token prefix and decodes the presigned URL it
// carries.
func decodeEksToken(t *testing.T, token string) *url.URL {
	t.Helper()

	require.True(
		t,
		strings.HasPrefix(token, EksTokenPrefix),
		"token must start with %s", EksTokenPrefix,
	)
	encodedUrl := strings.TrimPrefix(token, EksTokenPrefix)

	decodedUrl, err := base64.RawURLEncoding.DecodeString(encodedUrl)
	require.NoError(t, err, "token payload must be unpadded base64url")

	presignedUrl, err := url.Parse(string(decodedUrl))
	require.NoError(t, err)

	return presignedUrl
}

// TestGetEksTokenFormat checks that the generated token has the format the AWS
// IAM authenticator in the EKS control plane validates: the k8s-aws-v1. prefix
// followed by an unpadded base64url encoding of a presigned STS
// GetCallerIdentity URL.
func TestGetEksTokenFormat(t *testing.T) {
	token, _, err := GetEksToken(context.Background(), testAwsConfig(), "my-eks-cluster")
	require.NoError(t, err)

	// the authenticator decodes the payload with unpadded base64url - padding
	// characters would make it fail to decode
	assert.NotContains(t, token, "=", "token payload must not be padded")

	presignedUrl := decodeEksToken(t, token)

	assert.Equal(t, "https", presignedUrl.Scheme)
	assert.Equal(t, "sts.us-east-1.amazonaws.com", presignedUrl.Host)

	query := presignedUrl.Query()
	assert.Equal(t, "GetCallerIdentity", query.Get("Action"))
	assert.Equal(t, "AWS4-HMAC-SHA256", query.Get("X-Amz-Algorithm"))
	assert.NotEmpty(t, query.Get("X-Amz-Signature"))
	assert.NotEmpty(t, query.Get("X-Amz-Date"))
}

// TestGetEksTokenPresignExpires checks that the presigned URL carries a
// lifetime the authenticator accepts.  The AWS SDK does not set this
// parameter, and a presigned URL without it is rejected outright with
// "invalid X-Amz-Expires parameter in pre-signed URL: 0".
func TestGetEksTokenPresignExpires(t *testing.T) {
	token, _, err := GetEksToken(context.Background(), testAwsConfig(), "my-eks-cluster")
	require.NoError(t, err)

	presignedUrl := decodeEksToken(t, token)

	expires, err := strconv.Atoi(presignedUrl.Query().Get("X-Amz-Expires"))
	require.NoError(t, err, "presigned URL must carry an X-Amz-Expires parameter")
	assert.Greater(t, expires, 0)
	assert.LessOrEqual(t, expires, 900, "the authenticator rejects anything over 900 seconds")

	signedHeaders := presignedUrl.Query().Get("X-Amz-SignedHeaders")
	assert.NotEmpty(t, signedHeaders)
}

// TestGetEksTokenSignsClusterIdHeader checks that the cluster ID header is
// included in the request signature.  The EKS API server rejects a token whose
// presigned URL does not list x-k8s-aws-id among its signed headers, so an
// unsigned header would produce a token that looks correct but never
// authenticates.
func TestGetEksTokenSignsClusterIdHeader(t *testing.T) {
	token, _, err := GetEksToken(context.Background(), testAwsConfig(), "my-eks-cluster")
	require.NoError(t, err)

	presignedUrl := decodeEksToken(t, token)

	signedHeaders := strings.Split(presignedUrl.Query().Get("X-Amz-SignedHeaders"), ";")
	assert.Contains(
		t,
		signedHeaders,
		EksClusterIdHeader,
		"cluster ID header must be signed",
	)
}

// TestGetEksTokenIsClusterSpecific checks that the cluster name reaches the
// signature, i.e. that a token for one cluster cannot be replayed against
// another.
func TestGetEksTokenIsClusterSpecific(t *testing.T) {
	firstToken, _, err := GetEksToken(context.Background(), testAwsConfig(), "cluster-one")
	require.NoError(t, err)
	secondToken, _, err := GetEksToken(context.Background(), testAwsConfig(), "cluster-two")
	require.NoError(t, err)

	firstSignature := decodeEksToken(t, firstToken).Query().Get("X-Amz-Signature")
	secondSignature := decodeEksToken(t, secondToken).Query().Get("X-Amz-Signature")

	assert.NotEqual(
		t,
		firstSignature,
		secondSignature,
		"tokens for different clusters must have different signatures",
	)
}

// TestGetEksTokenExpiration checks that the reported expiration stays within
// the window AWS allows for these tokens.
func TestGetEksTokenExpiration(t *testing.T) {
	before := time.Now().UTC()
	_, expiration, err := GetEksToken(context.Background(), testAwsConfig(), "my-eks-cluster")
	require.NoError(t, err)
	after := time.Now().UTC()

	assert.False(t, expiration.Before(before.Add(EksTokenExpiration)))
	assert.False(t, expiration.After(after.Add(EksTokenExpiration)))
	// AWS invalidates the token 15 minutes after it is signed
	assert.Less(t, EksTokenExpiration, 15*time.Minute)
}

// TestEksConnectionRejectsNilConfig checks a nil AWS config is reported rather
// than dereferenced.  A caller that failed to resolve one reaches here with
// nil, and a segfault hides which caller it was.
func TestEksConnectionRejectsNilConfig(t *testing.T) {
	connectionInfo := EksClusterConnectionInfo{ClusterName: "some-cluster"}
	err := connectionInfo.Get(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without an AWS config")

	_, err = EksOidcIssuerUrl(nil, "some-cluster")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without an AWS config")
}

// TestEksTokenRoundTripsThroughEncryption checks a generated token survives
// the encrypt/decrypt round trip the teardown puts it through.  A token read
// back from the API arrives encrypted, so one refreshed locally has to be
// encrypted the same way or the reader fails decoding it rather than
// authenticating with it.
func TestEksTokenRoundTripsThroughEncryption(t *testing.T) {
	token, _, err := GetEksToken(context.Background(), testAwsConfig(), "my-eks-cluster")
	require.NoError(t, err)

	encryptionKey, err := encryption.GenerateKey()
	require.NoError(t, err)

	encryptedToken, err := encryption.Encrypt(encryptionKey, token)
	require.NoError(t, err)
	assert.NotEqual(t, token, encryptedToken)

	decryptedToken, err := encryption.Decrypt(encryptionKey, encryptedToken)
	require.NoError(t, err)
	assert.Equal(t, token, decryptedToken)
}
