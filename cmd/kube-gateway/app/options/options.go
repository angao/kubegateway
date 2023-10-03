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
	cliflag "k8s.io/component-base/cli/flag"
)

type Options struct {
	Proxy *ProxyOptions
}

func NewOptions() *Options {
	return &Options{
		Proxy: NewProxyOptions(),
	}
}

func (o *Options) Complete() error {
	return o.Proxy.Complete()
}

func (o *Options) Flags() cliflag.NamedFlagSets {
	return o.Proxy.Flags()
}

func (o *Options) Validate() []error {
	var errs []error
	errs = append(errs, o.Proxy.Validate()...)
	return errs
}
