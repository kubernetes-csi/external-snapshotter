/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package standardflags

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

type SidecarConfiguration struct {
	ShowVersion bool

	KubeConfig string
	CSIAddress string

	LeaderElection              bool
	LeaderElectionNamespace     string
	LeaderElectionLeaseDuration time.Duration
	LeaderElectionRenewDeadline time.Duration
	LeaderElectionRetryPeriod   time.Duration
	LeaderElectionLabels        stringMap

	KubeAPIQPS   float64
	KubeAPIBurst int

	HttpEndpoint   string
	MetricsAddress string
	MetricsPath    string
}

var Configuration = SidecarConfiguration{LeaderElectionLabels: make(stringMap)}

func RegisterCommonFlags(flags *flag.FlagSet) {
	flags.BoolVar(&Configuration.ShowVersion, "version", false, "Show version.")
	flags.StringVar(&Configuration.KubeConfig, "kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	flags.StringVar(&Configuration.CSIAddress, "csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	flags.BoolVar(&Configuration.LeaderElection, "leader-election", false, "Enable leader election.")
	flags.StringVar(&Configuration.LeaderElectionNamespace, "leader-election-namespace", "", "Namespace where the leader election resource lives. Defaults to the pod namespace if not set.")
	flags.DurationVar(&Configuration.LeaderElectionLeaseDuration, "leader-election-lease-duration", 15*time.Second, "Duration, in seconds, that non-leader candidates will wait to force acquire leadership. Defaults to 15 seconds.")
	flags.DurationVar(&Configuration.LeaderElectionRenewDeadline, "leader-election-renew-deadline", 10*time.Second, "Duration, in seconds, that the acting leader will retry refreshing leadership before giving up. Defaults to 10 seconds.")
	flags.DurationVar(&Configuration.LeaderElectionRetryPeriod, "leader-election-retry-period", 5*time.Second, "Duration, in seconds, the LeaderElector clients should wait between tries of actions. Defaults to 5 seconds.")
	flags.Var(&Configuration.LeaderElectionLabels, "leader-election-labels", "List of labels to add to lease when given replica becomes leader. Formatted as a comma seperated list of key:value labels. Example: 'my-label:my-value,my-second-label:my-second-value'")
	flags.Float64Var(&Configuration.KubeAPIQPS, "kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	flags.IntVar(&Configuration.KubeAPIBurst, "kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")
	flags.StringVar(&Configuration.HttpEndpoint, "http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080`). The default is empty string, which means the server is disabled. Only one of `--metrics-address` and `--http-endpoint` can be set.")
	flags.StringVar(&Configuration.MetricsAddress, "metrics-address", "", "(deprecated) The TCP network address where the prometheus metrics endpoint will listen (example: `:8080`). The default is empty string, which means metrics endpoint is disabled. Only one of `--metrics-address` and `--http-endpoint` can be set.")
	flag.StringVar(&Configuration.MetricsPath, "metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
}

type stringMap map[string]string

func (sm *stringMap) String() string {
	return fmt.Sprintf("%s", *sm)
}

func (sm *stringMap) Set(value string) error {
	outMap := *sm
	items := strings.Split(value, ",")
	for _, i := range items {
		label := strings.Split(i, ":")
		if len(label) != 2 {
			return fmt.Errorf("malformed item in list of labels: %s", i)
		}
		outMap[label[0]] = label[1]
	}
	return nil
}
