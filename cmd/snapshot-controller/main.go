/*
Copyright 2018 The Kubernetes Authors.

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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	klog "k8s.io/klog/v2"

	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	controller "github.com/kubernetes-csi/external-snapshotter/v3/pkg/common-controller"
	"github.com/kubernetes-csi/external-snapshotter/v3/pkg/metrics"

	clientset "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned"
	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/client/v3/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v3/informers/externalversions"
	coreinformers "k8s.io/client-go/informers"
)

// Command line flags
var (
	kubeconfig   = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	resyncPeriod = flag.Duration("resync-period", 60*time.Second, "Resync interval of the controller.")
	showVersion  = flag.Bool("version", false, "Show version.")
	threads      = flag.Int("worker-threads", 10, "Number of worker threads.")

	leaderElection          = flag.Bool("leader-election", false, "Enables leader election.")
	leaderElectionNamespace = flag.String("leader-election-namespace", "", "The namespace where the leader election resource exists. Defaults to the pod namespace if not set.")

	httpEndpoint = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics, will listen (example: :8080). The default is empty string, which means the server is disabled.")
	metricsPath  = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
)

var (
	version = "unknown"
)

func main() {
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}
	klog.Infof("Version: %s", version)

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	snapClient, err := clientset.NewForConfig(config)
	if err != nil {
		klog.Errorf("Error building snapshot clientset: %s", err.Error())
		os.Exit(1)
	}

	factory := informers.NewSharedInformerFactory(snapClient, *resyncPeriod)
	coreFactory := coreinformers.NewSharedInformerFactory(kubeClient, *resyncPeriod)

	// Create and register metrics manager
	metricsManager := metrics.NewMetricsManager()
	wg := &sync.WaitGroup{}
	wg.Add(1)
	if *httpEndpoint != "" {
		srv, err := metricsManager.StartMetricsEndpoint(*metricsPath, *httpEndpoint, promklog{}, wg)
		if err != nil {
			klog.Errorf("Failed to start metrics server: %s", err.Error())
			os.Exit(1)
		}
		defer func() {
			err := srv.Shutdown(context.Background())
			if err != nil {
				klog.Errorf("Failed to shutdown metrics server: %s", err.Error())
			}

			klog.Infof("Metrics server successfully shutdown")
			wg.Done()
		}()
		klog.Infof("Metrics server successfully started on %s, %s", *httpEndpoint, *metricsPath)
	}

	// Add Snapshot types to the default Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	klog.V(2).Infof("Start NewCSISnapshotController with kubeconfig [%s] resyncPeriod [%+v]", *kubeconfig, *resyncPeriod)

	ctrl := controller.NewCSISnapshotCommonController(
		snapClient,
		kubeClient,
		factory.Snapshot().V1beta1().VolumeSnapshots(),
		factory.Snapshot().V1beta1().VolumeSnapshotContents(),
		factory.Snapshot().V1beta1().VolumeSnapshotClasses(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		metricsManager,
		*resyncPeriod,
	)

	run := func(context.Context) {
		// run...
		stopCh := make(chan struct{})
		factory.Start(stopCh)
		coreFactory.Start(stopCh)
		go ctrl.Run(*threads, stopCh)

		// ...until SIGINT
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		close(stopCh)
	}

	if !*leaderElection {
		run(context.TODO())
	} else {
		lockName := "snapshot-controller-leader"
		// Create a new clientset for leader election to prevent throttling
		// due to snapshot controller
		leClientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.Fatalf("failed to create leaderelection client: %v", err)
		}
		le := leaderelection.NewLeaderElection(leClientset, lockName, run)
		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}
		if err := le.Run(); err != nil {
			klog.Fatalf("failed to initialize leader election: %v", err)
		}
	}
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

type promklog struct{}

func (pl promklog) Println(v ...interface{}) {
	klog.Error(v...)
}
