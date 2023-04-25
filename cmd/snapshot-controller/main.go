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
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	v1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	klog "k8s.io/klog/v2"

	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	controller "github.com/kubernetes-csi/external-snapshotter/v6/pkg/common-controller"
	"github.com/kubernetes-csi/external-snapshotter/v6/pkg/metrics"

	clientset "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v6/informers/externalversions"
	coreinformers "k8s.io/client-go/informers"
)

// Command line flags
var (
	kubeconfig   = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	resyncPeriod = flag.Duration("resync-period", 15*time.Minute, "Resync interval of the controller.")
	showVersion  = flag.Bool("version", false, "Show version.")
	threads      = flag.Int("worker-threads", 10, "Number of worker threads.")

	leaderElection              = flag.Bool("leader-election", false, "Enables leader election.")
	leaderElectionNamespace     = flag.String("leader-election-namespace", "", "The namespace where the leader election resource exists. Defaults to the pod namespace if not set.")
	leaderElectionLeaseDuration = flag.Duration("leader-election-lease-duration", 15*time.Second, "Duration, in seconds, that non-leader candidates will wait to force acquire leadership. Defaults to 15 seconds.")
	leaderElectionRenewDeadline = flag.Duration("leader-election-renew-deadline", 10*time.Second, "Duration, in seconds, that the acting leader will retry refreshing leadership before giving up. Defaults to 10 seconds.")
	leaderElectionRetryPeriod   = flag.Duration("leader-election-retry-period", 5*time.Second, "Duration, in seconds, the LeaderElector clients should wait between tries of actions. Defaults to 5 seconds.")

	kubeAPIQPS   = flag.Float64("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	kubeAPIBurst = flag.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")

	httpEndpoint                  = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics, will listen (example: :8080). The default is empty string, which means the server is disabled.")
	metricsPath                   = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
	retryIntervalStart            = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed volume snapshot creation or deletion. It doubles with each failure, up to retry-interval-max. Default is 1 second.")
	retryIntervalMax              = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed volume snapshot creation or deletion. Default is 5 minutes.")
	enableDistributedSnapshotting = flag.Bool("enable-distributed-snapshotting", false, "Enables each node to handle snapshotting for the local volumes created on that node")
	preventVolumeModeConversion   = flag.Bool("prevent-volume-mode-conversion", false, "Prevents an unauthorised user from modifying the volume mode when creating a PVC from an existing VolumeSnapshot.")
	enableVolumeGroupSnapshots    = flag.Bool("enable-volume-group-snapshots", false, "Enables the volume group snapshot feature, allowing the user to create snapshots of groups of volumes.")

	retryCRDIntervalMax = flag.Duration("retry-crd-interval-max", 5*time.Second, "Maximum retry interval to wait for CRDs to appear. The default is 5 seconds.")
)

var version = "unknown"

// Checks that the VolumeSnapshot v1 CRDs exist.
func ensureCustomResourceDefinitionsExist(client *clientset.Clientset) error {
	condition := func() (bool, error) {
		var err error

		// scoping to an empty namespace makes `List` work across all namespaces
		_, err = client.SnapshotV1().VolumeSnapshots("").List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Failed to list v1 volumesnapshots with error=%+v", err)
			return false, nil
		}

		_, err = client.SnapshotV1().VolumeSnapshotClasses().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Failed to list v1 volumesnapshotclasses with error=%+v", err)
			return false, nil
		}
		_, err = client.SnapshotV1().VolumeSnapshotContents().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Errorf("Failed to list v1 volumesnapshotcontents with error=%+v", err)
			return false, nil
		}
		return true, nil
	}

	// The maximum retry duration = initial duration * retry factor ^ # steps. Rearranging, this gives
	// # steps = log(maximum retry / initial duration) / log(retry factor).
	const retryFactor = 1.5
	const initialDurationMs = 100
	maxMs := retryCRDIntervalMax.Milliseconds()
	if maxMs < initialDurationMs {
		maxMs = initialDurationMs
	}
	steps := int(math.Ceil(math.Log(float64(maxMs)/initialDurationMs) / math.Log(retryFactor)))
	if steps < 1 {
		steps = 1
	}
	backoff := wait.Backoff{
		Duration: initialDurationMs * time.Millisecond,
		Factor:   retryFactor,
		Steps:    steps,
	}
	if err := wait.ExponentialBackoff(backoff, condition); err != nil {
		return err
	}

	return nil
}

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

	config.QPS = (float32)(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst

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
	var nodeInformer v1.NodeInformer

	if *enableDistributedSnapshotting {
		nodeInformer = coreFactory.Core().V1().Nodes()
	}

	// Create and register metrics manager
	metricsManager := metrics.NewMetricsManager()
	wg := &sync.WaitGroup{}

	mux := http.NewServeMux()
	if *httpEndpoint != "" {
		err := metricsManager.PrepareMetricsPath(mux, *metricsPath, promklog{})
		if err != nil {
			klog.Errorf("Failed to prepare metrics path: %s", err.Error())
			os.Exit(1)
		}
		klog.Infof("Metrics path successfully registered at %s", *metricsPath)
	}

	// Add Snapshot types to the default Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	klog.V(2).Infof("Start NewCSISnapshotController with kubeconfig [%s] resyncPeriod [%+v]", *kubeconfig, *resyncPeriod)

	ctrl := controller.NewCSISnapshotCommonController(
		snapClient,
		kubeClient,
		factory.Snapshot().V1().VolumeSnapshots(),
		factory.Snapshot().V1().VolumeSnapshotContents(),
		factory.Snapshot().V1().VolumeSnapshotClasses(),
		factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshots(),
		factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotContents(),
		factory.Groupsnapshot().V1alpha1().VolumeGroupSnapshotClasses(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		nodeInformer,
		metricsManager,
		*resyncPeriod,
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		workqueue.NewItemExponentialFailureRateLimiter(*retryIntervalStart, *retryIntervalMax),
		*enableDistributedSnapshotting,
		*preventVolumeModeConversion,
		*enableVolumeGroupSnapshots,
	)

	if err := ensureCustomResourceDefinitionsExist(snapClient); err != nil {
		klog.Errorf("Exiting due to failure to ensure CRDs exist during startup: %+v", err)
		os.Exit(1)
	}

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

	// start listening & serving http endpoint if set
	if *httpEndpoint != "" {
		l, err := net.Listen("tcp", *httpEndpoint)
		if err != nil {
			klog.Fatalf("failed to listen on address[%s], error[%v]", *httpEndpoint, err)
		}
		srv := &http.Server{Addr: l.Addr().String(), Handler: mux}
		go func() {
			defer wg.Done()
			if err := srv.Serve(l); err != http.ErrServerClosed {
				klog.Fatalf("failed to start endpoint at:%s/%s, error: %v", *httpEndpoint, *metricsPath, err)
			}
		}()
		klog.Infof("Metrics http server successfully started on %s, %s", *httpEndpoint, *metricsPath)

		defer func() {
			err := srv.Shutdown(context.Background())
			if err != nil {
				klog.Errorf("Failed to shutdown metrics server: %s", err.Error())
			}

			klog.Infof("Metrics server successfully shutdown")
			wg.Done()
		}()
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
		if *httpEndpoint != "" {
			le.PrepareHealthCheck(mux, leaderelection.DefaultHealthCheckTimeout)
		}

		if *leaderElectionNamespace != "" {
			le.WithNamespace(*leaderElectionNamespace)
		}
		le.WithLeaseDuration(*leaderElectionLeaseDuration)
		le.WithRenewDeadline(*leaderElectionRenewDeadline)
		le.WithRetryPeriod(*leaderElectionRetryPeriod)
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
