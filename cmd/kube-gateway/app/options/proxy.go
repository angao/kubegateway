// Copyright 2022 ByteDance and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package options

import (
	"fmt"
	"net"
	"os"

	genericoptions "k8s.io/apiserver/pkg/server/options"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
	"k8s.io/klog"

	proxyoptions "github.com/kubewharf/kubegateway/pkg/gateway/proxy/options"
)

type ProxyOptions struct {
	Authentication  *proxyoptions.AuthenticationOptions
	Authorization   *proxyoptions.AuthorizationOptions
	SecureServing   *proxyoptions.SecureServingOptions
	UpstreamCluster *proxyoptions.UpstreamClusterOptions
	ProcessInfo     *genericoptions.ProcessInfo
	Logging         *proxyoptions.LoggingOptions

	// FeatureGate is a way to plumb feature gate through if you have them.
	FeatureGate featuregate.FeatureGate
	Features    *genericoptions.FeatureOptions
	ServerRun   *genericoptions.ServerRunOptions
}

func NewProxyOptions() *ProxyOptions {
	return &ProxyOptions{
		Authentication:  proxyoptions.NewAuthenticationOptions(),
		Authorization:   proxyoptions.NewAuthorizationOptions(),
		SecureServing:   proxyoptions.NewSecureServingOptions(),
		UpstreamCluster: proxyoptions.NewUpstreamClusterOptions(),
		ProcessInfo:     genericoptions.NewProcessInfo("kube-gateway-proxy", "kube-system"),
		Logging:         proxyoptions.NewLoggingOptions(),
		FeatureGate:     featuregate.NewFeatureGate(),
		Features:        genericoptions.NewFeatureOptions(),
		ServerRun:       genericoptions.NewServerRunOptions(),
	}
}

// Flags returns flags for a proxy by section name
func (o *ProxyOptions) Flags() (fss cliflag.NamedFlagSets) {
	fs := fss.FlagSet("proxy")
	o.Authentication.AddFlags(fs)
	o.Authorization.AddFlags(fs)
	o.SecureServing.AddFlags(fs)
	o.UpstreamCluster.AddFlags(fs)
	o.Logging.AddFlags(fs)

	if o.Features != nil {
		o.Features.AddFlags(fss.FlagSet("features"))
	}

	if o.ServerRun != nil {
		o.ServerRun.AddUniversalFlags(fss.FlagSet("server run"))
	}
	return
}

func (o *ProxyOptions) Complete() error {
	if o.ServerRun != nil {
		if o.SecureServing != nil {
			if err := o.ServerRun.DefaultAdvertiseAddress(o.SecureServing.SecureServingOptions); err != nil {
				return err
			}
		}

		if len(o.ServerRun.ExternalHost) == 0 {
			if len(o.ServerRun.AdvertiseAddress) > 0 {
				o.ServerRun.ExternalHost = o.ServerRun.AdvertiseAddress.String()
			} else {
				if hostname, err := os.Hostname(); err == nil {
					o.ServerRun.ExternalHost = hostname
				} else {
					return fmt.Errorf("error finding host name: %v", err)
				}
			}
			klog.Infof("external host was not specified, using %v", o.ServerRun.ExternalHost)
		}
	}

	if o.SecureServing != nil {
		if err := o.SecureServing.MaybeDefaultWithSelfSignedCerts(o.ServerRun.AdvertiseAddress.String(), []string{"kubernetes.default.svc", "kubernetes.default", "kubernetes"}, []net.IP{}); err != nil {
			return fmt.Errorf("error creating self-signed certificates: %v", err)
		}
	}
	return nil
}

func (o *ProxyOptions) Validate() []error {
	var errs []error
	errs = append(errs, o.Authentication.Validate()...)
	errs = append(errs, o.Authorization.Validate()...)
	errs = append(errs, o.SecureServing.Validate()...)
	errs = append(errs, o.UpstreamCluster.Validate()...)

	if o.Features != nil {
		errs = append(errs, o.Features.Validate()...)
	}

	if o.ServerRun != nil {
		errs = append(errs, o.ServerRun.Validate()...)
	}

	return errs
}
