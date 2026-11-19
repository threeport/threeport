package handlers

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	echo "github.com/labstack/echo/v4"
	zap "go.uber.org/zap"

	apiserver_lib "github.com/threeport/threeport/pkg/api-server/lib/v0"
	api_lib "github.com/threeport/threeport/pkg/api/lib/v0"
	api_v0 "github.com/threeport/threeport/pkg/api/v0"
	auth "github.com/threeport/threeport/pkg/auth/v0"
	util "github.com/threeport/threeport/pkg/util/v0"
)

// signingCaDir is where the API pod has the control plane's CA certificate and
// private key mounted.  The API is the one place the key lives.  It is a
// variable only so a test can point it at a directory of its own.
var signingCaDir = "/etc/threeport/ca"

const (
	// maxCertificateRequestLength bounds the request a caller may send.  A
	// request for a 4096-bit key is under 1 KiB.
	maxCertificateRequestLength = 16 * 1024
)

// CreateAgentCertificate issues a client certificate to the agent on a managed
// runtime.
//
// The kubernetes runtime controller installs an agent on every managed cluster
// and has to give it a client certificate.  It asks the API for one, rather
// than holding the CA's private key itself, so that a compromised controller
// can neither take the key nor sign a certificate with any name it likes: the
// API picks the identity, and the controller only supplies the agent's public
// key.
//
// @Summary issues a client certificate to the agent on a managed runtime.
// @Description Signs the public key in a certificate signing request with the control plane's CA.  The certificate's common name is derived from the runtime instance name, and its organization and organizational unit are set by the API; the request's own subject is ignored.  Only the kubernetes runtime controller may call this, and only for a runtime instance that exists.  Available only when the control plane has authentication enabled.
// @ID create-v0-agentCertificate
// @Accept json
// @Produce json
// @Param agentCertificateRequest body api_v0.AgentCertificateRequest true "AgentCertificateRequest object"
// @Success 201 {object} v0.Response "Created"
// @Failure 400 {object} v0.Response "Bad Request"
// @Failure 403 {object} v0.Response "Forbidden"
// @Failure 404 {object} v0.Response "Not Found"
// @Failure 409 {object} v0.Response "Conflict"
// @Failure 500 {object} v0.Response "Internal Server Error"
// @Router /v0/agent-certificates [POST]
func (h Handler) CreateAgentCertificate(c echo.Context) error {
	fullyQualifiedType := new(api_v0.AgentCertificate).GetFullyQualifiedType()

	var request api_v0.AgentCertificateRequest
	if err := c.Bind(&request); err != nil {
		h.Logger.Error("handler error: error binding agent certificate request", zap.Error(err))
		return apiserver_lib.ResponseStatusBindErr(c, nil, err, fullyQualifiedType)
	}

	caller := api_lib.Caller(c.Request().Context())
	certificate, err := issueAgentCertificate(
		c.Request().Context(),
		signingCaDir,
		caller,
		request,
		h.runtimeInstanceExists,
		h.Logger,
	)
	if err != nil {
		var httpErr *util.HttpError
		if errors.As(err, &httpErr) {
			h.Logger.Info(
				"handler error: agent certificate refused",
				zap.String("caller", caller.CommonName),
				zap.Int("status", httpErr.GetStatusCode()),
				zap.Error(err),
			)
			return apiserver_lib.ResponseStatusErr(httpErr.GetStatusCode(), c, nil, err, fullyQualifiedType)
		}
		h.Logger.Error("handler error: error issuing agent certificate", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, errors.New("failed to issue agent certificate"), fullyQualifiedType)
	}

	response, err := apiserver_lib.CreateResponse(apiserver_lib.SingleObjectMeta(), *certificate, fullyQualifiedType)
	if err != nil {
		h.Logger.Error("handler error: error creating response", zap.Error(err))
		return apiserver_lib.ResponseStatus500(c, nil, err, fullyQualifiedType)
	}

	return apiserver_lib.ResponseStatus201(c, *response)
}

// runtimeInstanceExists reports whether a kubernetes runtime instance with the
// name exists.
func (h Handler) runtimeInstanceExists(ctx context.Context, name string) (bool, error) {
	var count int64
	if err := h.DB.WithContext(ctx).
		Model(&api_v0.KubernetesRuntimeInstance{}).
		Where("name = ?", name).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to look up kubernetes runtime instance %q: %w", name, err)
	}

	return count > 0, nil
}

// issueAgentCertificate decides whether the caller may have an agent
// certificate for the request, and signs it.  The CA is read from caDir on
// every call rather than held, so a rotated CA is picked up and the key is in
// this process's memory only while a certificate is being signed.
//
// Errors the caller can act on are *util.HttpError.  Any other error is a
// fault of the server's and is not reported to the caller in detail.
func issueAgentCertificate(
	ctx context.Context,
	caDir string,
	caller api_lib.CallerIdentity,
	request api_v0.AgentCertificateRequest,
	runtimeExists func(ctx context.Context, name string) (bool, error),
	logger *zap.Logger,
) (*api_v0.AgentCertificate, error) {
	authConfig, err := loadSigningCa(caDir)
	if err != nil {
		return nil, err
	}

	// Auth is organizational-unit based, and every control plane component has
	// the same unit, so the unit alone would let any of them have a certificate.
	// Only the one that installs agents may.
	if caller.CommonName != auth.CNKubernetesRuntimeController ||
		caller.OrganizationalUnit != auth.OUControlPlane {
		return nil, util.NewForbiddenError("only the kubernetes runtime controller may request an agent certificate")
	}

	if err := auth.ValidateRuntimeName(request.RuntimeInstanceName); err != nil {
		return nil, util.NewBadRequestError(err.Error())
	}
	if request.CertificateSigningRequest == "" {
		return nil, util.NewBadRequestError("certificate signing request is empty")
	}
	if len(request.CertificateSigningRequest) > maxCertificateRequestLength {
		return nil, util.NewBadRequestError("certificate signing request is too large")
	}

	exists, err := runtimeExists(ctx, request.RuntimeInstanceName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, util.NewNotFoundError(fmt.Sprintf(
			"kubernetes runtime instance %q not found", request.RuntimeInstanceName,
		))
	}

	commonName := auth.AgentCommonName(request.RuntimeInstanceName)
	certificate, serial, err := auth.SignClientCertificateRequest(
		authConfig,
		request.CertificateSigningRequest,
		commonName,
		api_lib.CoreApiNamespace,
		auth.OUControlPlane,
	)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCertificateRequest) {
			return nil, util.NewBadRequestError(err.Error())
		}
		return nil, fmt.Errorf("failed to sign agent certificate: %w", err)
	}

	// every certificate the API signs is on record
	logger.Info(
		"issued agent certificate",
		zap.String("commonName", commonName),
		zap.String("serialNumber", serial),
		zap.String("runtimeInstance", request.RuntimeInstanceName),
		zap.String("requestedBy", caller.CommonName),
	)

	return &api_v0.AgentCertificate{
		Certificate:   certificate,
		CACertificate: authConfig.CAPemEncoded,
	}, nil
}

// loadSigningCa reads the control plane's CA certificate and private key from
// dir.  Neither being there means the control plane runs without
// authentication, which is not a fault: it has no client certificates to issue,
// and says so with a conflict.  One without the other, or a pair that does not
// load, is a fault of the server's.
func loadSigningCa(dir string) (*auth.AuthConfig, error) {
	caPem, certErr := os.ReadFile(filepath.Join(dir, "tls.crt"))
	caKeyPem, keyErr := os.ReadFile(filepath.Join(dir, "tls.key"))

	if errors.Is(certErr, fs.ErrNotExist) && errors.Is(keyErr, fs.ErrNotExist) {
		return nil, util.NewConflictError("this control plane has authentication disabled, so it issues no certificates")
	}
	if certErr != nil {
		return nil, fmt.Errorf("failed to read the signing CA certificate: %w", certErr)
	}
	if keyErr != nil {
		return nil, fmt.Errorf("failed to read the signing CA private key: %w", keyErr)
	}

	authConfig, err := auth.LoadAuthConfig(caPem, caKeyPem)
	if err != nil {
		return nil, fmt.Errorf("failed to load the signing CA: %w", err)
	}

	return authConfig, nil
}
