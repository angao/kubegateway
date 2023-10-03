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
	"github.com/spf13/pflag"
	"k8s.io/apiserver/pkg/server"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/rest"
)

type SecureServingOptions struct {
	*genericoptions.SecureServingOptionsWithLoopback
}

func NewSecureServingOptions() *SecureServingOptions {
	sso := genericoptions.NewSecureServingOptions()

	// We are composing recommended options for an aggregated api-server,
	// whose client is typically a proxy multiplexing many operations ---
	// notably including long-running ones --- into one HTTP/2 connection
	// into this server.  So allow many concurrent operations.
	sso.HTTP2MaxStreamsPerConnection = 1000
	return &SecureServingOptions{
		SecureServingOptionsWithLoopback: sso.WithLoopback(),
	}
}

func (s *SecureServingOptions) Validate() []error {
	if s == nil {
		return nil
	}

	var errs []error
	errs = append(errs, s.SecureServingOptionsWithLoopback.Validate()...)
	return errs
}

func (s *SecureServingOptions) AddFlags(fs *pflag.FlagSet) {
	if s == nil {
		return
	}
	s.SecureServingOptions.AddFlags(fs)
}

func (s *SecureServingOptions) ApplyTo(secureServingInfo **server.SecureServingInfo, loopbackClientConfig **rest.Config) error {
	if s == nil || s.SecureServingOptionsWithLoopback == nil || s.SecureServingOptions == nil || secureServingInfo == nil {
		return nil
	}

	if err := s.SecureServingOptionsWithLoopback.ApplyTo(secureServingInfo, loopbackClientConfig); err != nil {
		return err
	}
	return nil
}
