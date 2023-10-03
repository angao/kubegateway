// Copyright 2022 ByteDance and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package options

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/authentication/request/x509"
	genericserver "k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	openapicommon "k8s.io/kube-openapi/pkg/common"

	"github.com/kubewharf/kubegateway/pkg/clusters"
	proxyauthenticator "github.com/kubewharf/kubegateway/pkg/gateway/proxy/authenticator"
)

type AuthenticationOptions struct {
	APIAudiences  []string
	ClientCert    *genericoptions.ClientCertAuthenticationOptions
	RequestHeader *genericoptions.RequestHeaderAuthenticationOptions

	TokenSuccessCacheTTL time.Duration
	TokenFailureCacheTTL time.Duration
}

func NewAuthenticationOptions() *AuthenticationOptions {
	o := &AuthenticationOptions{
		ClientCert:           &genericoptions.ClientCertAuthenticationOptions{},
		RequestHeader:        &genericoptions.RequestHeaderAuthenticationOptions{},
		TokenSuccessCacheTTL: 600 * time.Second, // 10 minutes
		TokenFailureCacheTTL: 10 * time.Second,
	}
	return o
}

func (o *AuthenticationOptions) Validate() []error {
	var errs []error
	if o.RequestHeader != nil {
		errs = append(errs, o.RequestHeader.Validate()...)
	}
	return errs
}

func (o *AuthenticationOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&o.APIAudiences, "api-audiences", o.APIAudiences, ""+
		"Identifiers of the API. The service account token authenticator will validate that "+
		"tokens used against the API are bound to at least one of these audiences. If the "+
		"--service-account-issuer flag is configured and this flag is not, this field "+
		"defaults to a single element list containing the issuer URL.")

	if o.ClientCert != nil {
		o.ClientCert.AddFlags(fs)
	}

	if o.RequestHeader != nil {
		o.RequestHeader.AddFlags(fs)
	}

	fs.DurationVar(&o.TokenSuccessCacheTTL, "authentication-token-success-cache-ttl", o.TokenSuccessCacheTTL,
		"The duration to cache success responses from the upstream token request authenticator.")
	fs.DurationVar(&o.TokenFailureCacheTTL, "authentication-token-failure-cache-ttl", o.TokenFailureCacheTTL,
		"The duration to cache failure responses from the upstream token request authenticator.")
}

func (o *AuthenticationOptions) ToAuthenticationConfig(
	sniVerifyOptionsProvider x509.SNIVerifyOptionsProvider,
	clientProvider clusters.ClientProvider,
) (*proxyauthenticator.AuthenricatorConfig, error) {
	if o == nil {
		return nil, nil
	}
	cfg := proxyauthenticator.AuthenricatorConfig{
		TokenSuccessCacheTTL: o.TokenSuccessCacheTTL,
		TokenFailureCacheTTL: o.TokenFailureCacheTTL,
		APIAudiences:         o.APIAudiences,
		Anonymous:            true,
	}

	if o.ClientCert != nil {
		clientCAContentProvider, err := o.ClientCert.GetClientCAContentProvider()
		if err != nil {
			return nil, err
		}

		cfg.ClientCert = &proxyauthenticator.ClientCertAuthenticationConfig{
			CAContentProvider: clientCAContentProvider,
		}
	}

	if o.RequestHeader != nil {
		requestHeader, err := o.RequestHeader.ToAuthenticationRequestHeaderConfig()
		if err != nil {
			return nil, err
		}
		cfg.RequestHeaderConfig = requestHeader
	}

	if sniVerifyOptionsProvider != nil {
		// dynamic sni verify options provider
		if cfg.ClientCert == nil {
			cfg.ClientCert = &proxyauthenticator.ClientCertAuthenticationConfig{}
		}
		cfg.ClientCert.SNIVerifyOptionsPorvider = sniVerifyOptionsProvider
	}

	if clientProvider != nil {
		cfg.TokenRequest = &proxyauthenticator.TokenAuthenticationConfig{
			ClusterClientProvider: clientProvider,
		}
	}

	return &cfg, nil
}

func (o *AuthenticationOptions) ApplyTo(
	authenticationInfo *genericserver.AuthenticationInfo,
	servingInfo *genericserver.SecureServingInfo,
	openAPIConfig *openapicommon.Config,
	sniVerifyOptionsProvider x509.SNIVerifyOptionsProvider,
	clientProvider clusters.ClientProvider,
) error {
	if o == nil {
		authenticationInfo.Authenticator = nil
		return nil
	}

	cfg, err := o.ToAuthenticationConfig(sniVerifyOptionsProvider, clientProvider)
	if err != nil {
		return err
	}

	// get the clientCA information
	if cfg.ClientCert != nil && cfg.ClientCert.CAContentProvider != nil {
		if err := authenticationInfo.ApplyClientCert(cfg.ClientCert.CAContentProvider, servingInfo); err != nil {
			return fmt.Errorf("unable to assign client CA file: %v", err)
		}
	}

	if cfg.RequestHeaderConfig != nil {
		if err := authenticationInfo.ApplyClientCert(cfg.RequestHeaderConfig.CAContentProvider, servingInfo); err != nil {
			return fmt.Errorf("unable to load request-header-client-ca-file: %v", err)
		}
	}

	authenticationInfo.APIAudiences = cfg.APIAudiences

	// create authenticator
	req, securityDefinitions, err := cfg.New()
	if err != nil {
		return err
	}
	authenticationInfo.Authenticator = req
	if openAPIConfig != nil {
		openAPIConfig.SecurityDefinitions = securityDefinitions
	}
	authenticationInfo.SupportsBasicAuth = false

	return nil
}
