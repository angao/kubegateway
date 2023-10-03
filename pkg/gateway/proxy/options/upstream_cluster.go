package options

import (
	"fmt"

	"github.com/spf13/pflag"
)

type UpstreamClusterOptions struct {
	Path string
}

func NewUpstreamClusterOptions() *UpstreamClusterOptions {
	return &UpstreamClusterOptions{}
}

func (s *UpstreamClusterOptions) Validate() []error {
	if s == nil {
		return nil
	}

	var errs []error
	if len(s.Path) == 0 {
		errs = append(errs, fmt.Errorf("--upstream-cluster-file must be set"))
	}
	return errs
}

func (s *UpstreamClusterOptions) AddFlags(fs *pflag.FlagSet) {
	if s == nil {
		return
	}
	fs.StringVar(&s.Path, "upstream-cluster-file", s.Path, "File contains the upstream cluster configuration.")
}
