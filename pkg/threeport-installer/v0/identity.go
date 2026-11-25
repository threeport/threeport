package v0

import "errors"

// ValidateInstallIdentity checks the namespace, CA, and encryption-key options for a new install.
// An empty option keeps that option's default.
func ValidateInstallIdentity(opts Options) error {
	// reject a source namespace that is the install itself
	if opts.FromNamespace != "" && opts.FromNamespace == opts.Namespace {
		return errors.New("from-namespace is the namespace being installed")
	}

	// reject a cluster copy combined with files
	if opts.FromNamespace != "" && (opts.CACertFile != "" || opts.CAKeyFile != "" || opts.EncryptionKeyFile != "") {
		return errors.New("from-namespace cannot be combined with ca or encryption-key files")
	}

	// reject a CA cert without its key, and the reverse
	if (opts.CACertFile == "") != (opts.CAKeyFile == "") {
		return errors.New("ca cert and ca key must both be set or both be empty")
	}

	// reject a CA when auth will not use it
	if !opts.AuthEnabled && opts.CACertFile != "" {
		return errors.New("ca files are unused when auth is disabled")
	}

	return nil
}
