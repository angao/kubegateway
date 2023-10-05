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

// Package app does all of the work necessary to create a Kubernetes
// APIServer by binding together the API, master and APIServer infrastructure.
// It can be configured and called directly or via the hyperkube framework.
package app

import (
	"fmt"

	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/util/term"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog"
	utilflag "k8s.io/kubernetes/pkg/util/flag"

	"github.com/kubewharf/kubegateway/cmd/kube-gateway/app/options"
	"github.com/kubewharf/kubegateway/pkg/version"
)

// NewKubeGatewayCommand creates a *cobra.Command object with default parameters
func NewKubeGatewayCommand() *cobra.Command {
	s := options.NewOptions()
	cmd := &cobra.Command{
		Use: "kube-gateway",
		Long: `The Kubernetes API server validates and configures data
for the api objects which include pods, services and others. 
The API Server services REST operations and provides the frontend to the
cluster's shared state through which all other components interact.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()
			utilflag.PrintFlags(cmd.Flags())

			// set default options
			if err := s.Complete(); err != nil {
				return err
			}

			// validate options
			if errs := s.Validate(); len(errs) != 0 {
				return utilerrors.NewAggregate(errs)
			}

			return Run(s, genericapiserver.SetupSignalHandler())
		},
		SilenceUsage: true,
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		_, _ = fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols)
		return nil
	})

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols)
	})

	return cmd
}

// Run runs the specified APIServer.  This should never exit.
func Run(completeOptions *options.Options, stopCh <-chan struct{}) error {
	// To help debugging, immediately log version
	klog.Infof("Version: %+v", version.Get())

	cfg, err := CreateProxyConfig(completeOptions.Proxy)
	if err != nil {
		return err
	}

	server, err := cfg.Complete().New(genericapiserver.NewEmptyDelegate())
	if err != nil {
		return err
	}

	prepared := server.PrepareRun()
	return prepared.Run(stopCh)
}
