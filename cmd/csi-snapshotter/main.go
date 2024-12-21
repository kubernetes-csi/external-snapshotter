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
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"google.golang.org/grpc"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	coreinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	klog "k8s.io/klog/v2"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/connection"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/metrics"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/features"
	controller "github.com/kubernetes-csi/external-snapshotter/v8/pkg/sidecar-controller"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/snapshotter"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	_ "k8s.io/component-base/logs/json/register"

	clientset "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/group_snapshotter"
	utils "github.com/kubernetes-csi/external-snapshotter/v8/pkg/utils"
)

const (
	// Default timeout of short CSI calls like GetPluginInfo
	defaultCSITimeout = time.Minute
)

// Command line flags
var (
	kubeconfig             = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	csiAddress             = flag.String("csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	resyncPeriod           = flag.Duration("resync-period", 15*time.Minute, "Resync interval of the controller. Default is 15 minutes")
	snapshotNamePrefix     = flag.String("snapshot-name-prefix", "snapshot", "Prefix to apply to the name of a created snapshot")
	snapshotNameUUIDLength = flag.Int("snapshot-name-uuid-length", -1, "Length in characters for the generated uuid of a created snapshot. Defaults behavior is to NOT truncate.")
	showVersion            = flag.Bool("version", false, "Show version.")
	threads                = flag.Int("worker-threads", 10, "Number of worker threads.")
	csiTimeout             = flag.Duration("timeout", defaultCSITimeout, "The timeout for any RPCs to the CSI driver. Default is 1 minute.")
	extraCreateMetadata    = flag.Bool("extra-create-metadata", false, "If set, add snapshot metadata to plugin snapshot requests as parameters.")

	leaderElection              = flag.Bool("leader-election", false, "Enables leader election.")
	leaderElectionNamespace     = flag.String("leader-election-namespace", "", "The namespace where the leader election resource exists. Defaults to the pod namespace if not set.")
	leaderElectionLeaseDuration = flag.Duration("leader-election-lease-duration", 15*time.Second, "Duration, in seconds, that non-leader candidates will wait to force acquire leadership. Defaults to 15 seconds.")
	leaderElectionRenewDeadline = flag.Duration("leader-election-renew-deadline", 10*time.Second, "Duration, in seconds, that the acting leader will retry refreshing leadership before giving up. Defaults to 10 seconds.")
	leaderElectionRetryPeriod   = flag.Duration("leader-election-retry-period", 5*time.Second, "Duration, in seconds, the LeaderElector clients should wait between tries of actions. Defaults to 5 seconds.")

	kubeAPIQPS   = flag.Float64("kube-api-qps", 5, "QPS to use while communicating with the kubernetes apiserver. Defaults to 5.0.")
	kubeAPIBurst = flag.Int("kube-api-burst", 10, "Burst to use while communicating with the kubernetes apiserver. Defaults to 10.")

	metricsAddress              = flag.String("metrics-address", "", "(deprecated) The TCP network address where the prometheus metrics endpoint will listen (example: `:8080`). The default is empty string, which means metrics endpoint is disabled. Only one of `--metrics-address` and `--http-endpoint` can be set.")
	httpEndpoint                = flag.String("http-endpoint", "", "The TCP network address where the HTTP server for diagnostics, including metrics and leader election health check, will listen (example: `:8080`). The default is empty string, which means the server is disabled. Only one of `--metrics-address` and `--http-endpoint` can be set.")
	metricsPath                 = flag.String("metrics-path", "/metrics", "The HTTP path where prometheus metrics will be exposed. Default is `/metrics`.")
	retryIntervalStart          = flag.Duration("retry-interval-start", time.Second, "Initial retry interval of failed volume snapshot creation or deletion. It doubles with each failure, up to retry-interval-max. Default is 1 second.")
	retryIntervalMax            = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval of failed volume snapshot creation or deletion. Default is 5 minutes.")
	enableNodeDeployment        = flag.Bool("node-deployment", false, "Enables deploying the sidecar controller together with a CSI driver on nodes to manage snapshots for node-local volumes.")
	groupSnapshotNamePrefix     = flag.String("groupsnapshot-name-prefix", "groupsnapshot", "Prefix to apply to the name of a created group snapshot")
	groupSnapshotNameUUIDLength = flag.Int("groupsnapshot-name-uuid-length", -1, "Length in characters for the generated uuid of a created group snapshot. Defaults behavior is to NOT truncate.")
	featureGates                map[string]bool
)

var (
	version = "unknown"
	prefix  = "external-snapshotter-leader"
)

func main() {
	flag.Var(utilflag.NewMapStringBool(&featureGates), "feature-gates", "Comma-seprated list of key=value pairs that describe feature gates for alpha/experimental features. "+
		"Options are:\n"+strings.Join(utilfeature.DefaultFeatureGate.KnownFeatures(), "\n"))

	fg := featuregate.NewFeatureGate()
	logsapi.AddFeatureGates(fg)
	c := logsapi.NewLoggingConfiguration()
	logsapi.AddGoFlags(c, flag.CommandLine)
	logs.InitLogs()
	flag.Parse()
	if err := logsapi.ValidateAndApply(c, fg); err != nil {
		klog.ErrorS(err, "LoggingConfiguration is invalid")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}

	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(featureGates); err != nil {
		klog.Fatal("Error while parsing feature gates: ", err)
	}

	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}
	klog.InfoS("Version", "version", version)

	// If distributed snapshotting is enabled and leaderElection is also set to true, return
	if *enableNodeDeployment && *leaderElection {
		klog.Error("Leader election cannot happen when node-deployment is set to true")
		os.Exit(1)
	}

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	config.QPS = (float32)(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst

	coreConfig := rest.CopyConfig(config)
	coreConfig.ContentType = runtime.ContentTypeProtobuf
	kubeClient, err := kubernetes.NewForConfig(coreConfig)
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
	var snapshotContentfactory informers.SharedInformerFactory
	if *enableNodeDeployment {
		node := os.Getenv("NODE_NAME")
		if node == "" {
			klog.Fatal("The NODE_NAME environment variable must be set when using --enable-node-deployment.")
		}
		snapshotContentfactory = informers.NewSharedInformerFactoryWithOptions(snapClient, *resyncPeriod, informers.WithTweakListOptions(func(lo *v1.ListOptions) {
			lo.LabelSelector = labels.Set{utils.VolumeSnapshotContentManagedByLabel: node}.AsSelector().String()
		}),
		)
	} else {
		snapshotContentfactory = factory
	}

	// Add Snapshot types to the default Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	if *metricsAddress != "" && *httpEndpoint != "" {
		klog.Error("only one of `--metrics-address` and `--http-endpoint` can be set.")
		os.Exit(1)
	}
	addr := *metricsAddress
	if addr == "" {
		addr = *httpEndpoint
	}

	// Connect to CSI.
	metricsManager := metrics.NewCSIMetricsManager("" /* driverName */)
	ctx := context.Background()
	csiConn, err := connection.Connect(
		ctx,
		*csiAddress,
		metricsManager,
		connection.OnConnectionLoss(connection.ExitOnConnectionLoss()))
	if err != nil {
		klog.Errorf("error connecting to CSI driver: %v", err)
		os.Exit(1)
	}

	// Pass a context with a timeout
	tctx, cancel := context.WithTimeout(ctx, *csiTimeout)
	defer cancel()

	// Find driver name
	driverName, err := csirpc.GetDriverName(tctx, csiConn)
	if err != nil {
		klog.Errorf("error getting CSI driver name: %v", err)
		os.Exit(1)
	}

	klog.V(2).Infof("CSI driver name: %q", driverName)

	// Prepare http endpoint for metrics + leader election healthz
	mux := http.NewServeMux()
	if addr != "" {
		metricsManager.RegisterToServer(mux, *metricsPath)
		metricsManager.SetDriverName(driverName)
		go func() {
			klog.Infof("ServeMux listening at %q", addr)
			err := http.ListenAndServe(addr, mux)
			if err != nil {
				klog.Fatalf("Failed to start HTTP server at specified address (%q) and metrics path (%q): %s", addr, *metricsPath, err)
			}
		}()
	}

	// Check it's ready
	if err = csirpc.ProbeForever(ctx, csiConn, *csiTimeout); err != nil {
		klog.Errorf("error waiting for CSI driver to be ready: %v", err)
		os.Exit(1)
	}

	// Find out if the driver supports create/delete snapshot.
	tctx, cancel = context.WithTimeout(ctx, *csiTimeout)
	defer cancel()
	supportsCreateSnapshot, err := supportsControllerCreateSnapshot(tctx, csiConn)
	if err != nil {
		klog.Errorf("error determining if driver supports create/delete snapshot operations: %v", err)
		os.Exit(1)
	}
	if !supportsCreateSnapshot {
		klog.Errorf("CSI driver %s does not support ControllerCreateSnapshot", driverName)
		os.Exit(1)
	}

	if len(*snapshotNamePrefix) == 0 {
		klog.Error("Snapshot name prefix cannot be of length 0")
		os.Exit(1)
	}

	klog.V(2).Infof("Start NewCSISnapshotSideCarController with snapshotter [%s] kubeconfig [%s] csiTimeout [%+v] csiAddress [%s] resyncPeriod [%+v] snapshotNamePrefix [%s] snapshotNameUUIDLength [%d]", driverName, *kubeconfig, *csiTimeout, *csiAddress, *resyncPeriod, *snapshotNamePrefix, snapshotNameUUIDLength)

	snapShotter := snapshotter.NewSnapshotter(csiConn)
	var groupSnapshotter group_snapshotter.GroupSnapshotter
	if utilfeature.DefaultFeatureGate.Enabled(features.VolumeGroupSnapshot) {
		tctx, cancel = context.WithTimeout(ctx, *csiTimeout)
		defer cancel()
		supportsCreateVolumeGroupSnapshot, err := supportsGroupControllerCreateVolumeGroupSnapshot(tctx, csiConn)
		if err != nil {
			klog.Errorf("error determining if driver supports create/delete group snapshot operations: %v", err)
		} else if !supportsCreateVolumeGroupSnapshot {
			klog.Warningf("CSI driver %s does not support GroupControllerCreateVolumeGroupSnapshot when the --feature-gates=CSIVolumeGroupSnapshot=true flag is set", driverName)
		}
		groupSnapshotter = group_snapshotter.NewGroupSnapshotter(csiConn)
		if len(*groupSnapshotNamePrefix) == 0 {
			klog.Error("group snapshot name prefix cannot be of length 0")
			os.Exit(1)
		}
	}

	ctrl := controller.NewCSISnapshotSideCarController(
		snapClient,
		kubeClient,
		driverName,
		snapshotContentfactory.Snapshot().V1().VolumeSnapshotContents(),
		factory.Snapshot().V1().VolumeSnapshotClasses(),
		snapShotter,
		groupSnapshotter,
		*csiTimeout,
		*resyncPeriod,
		*snapshotNamePrefix,
		*snapshotNameUUIDLength,
		*groupSnapshotNamePrefix,
		*groupSnapshotNameUUIDLength,
		*extraCreateMetadata,
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](*retryIntervalStart, *retryIntervalMax),
		utilfeature.DefaultFeatureGate.Enabled(features.VolumeGroupSnapshot),
		snapshotContentfactory.Groupsnapshot().V1beta1().VolumeGroupSnapshotContents(),
		snapshotContentfactory.Groupsnapshot().V1beta1().VolumeGroupSnapshotClasses(),
		workqueue.NewTypedItemExponentialFailureRateLimiter[string](*retryIntervalStart, *retryIntervalMax),
	)

	run := func(context.Context) {
		// run...
		stopCh := make(chan struct{})
		snapshotContentfactory.Start(stopCh)
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
		lockName := fmt.Sprintf("%s-%s", prefix, strings.Replace(driverName, "/", "-", -1))
		// Create a new clientset for leader election to prevent throttling
		// due to snapshot sidecar
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

func supportsControllerCreateSnapshot(ctx context.Context, conn *grpc.ClientConn) (bool, error) {
	capabilities, err := csirpc.GetControllerCapabilities(ctx, conn)
	if err != nil {
		return false, err
	}

	return capabilities[csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT], nil
}

func supportsGroupControllerCreateVolumeGroupSnapshot(ctx context.Context, conn *grpc.ClientConn) (bool, error) {
	capabilities, err := csirpc.GetGroupControllerCapabilities(ctx, conn)
	if err != nil {
		return false, err
	}

	return capabilities[csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT], nil
}
