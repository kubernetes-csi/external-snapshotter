/*
Copyright 2026 The Kubernetes Authors.

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
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	"github.com/kubernetes-csi/csi-lib-utils/standardflags"
	clientset "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	snapinformers "github.com/kubernetes-csi/external-snapshotter/client/v8/informers/externalversions"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/featuregate"
	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/features"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topology"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topologycontroller"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/topologywebhook"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/webhook"
)

var (
	kubeconfig   = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	resyncPeriod = flag.Duration("resync-period", 15*time.Minute, "Resync interval of the informers.")
	showVersion  = flag.Bool("version", false, "Show version.")
	threads      = flag.Int("worker-threads", 10, "Number of controller worker threads.")

	certFile = flag.String("tls-cert-file", "", "File containing the x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert). Required.")
	keyFile  = flag.String("tls-private-key-file", "", "File containing the x509 private key matching --tls-cert-file. Required.")
	port     = flag.Int("port", 443, "Secure port that the webhooks listen on.")

	shutdownDelay    = flag.Duration("shutdown-delay", 10*time.Second, "How long to keep serving after SIGTERM while reporting unready, so the Service stops routing admission requests to this replica first.")
	maxAffinityTerms = flag.Int("max-affinity-terms", 32, "Maximum number of node selector terms in a pod's merged required node affinity. Must be positive.")
	gateTimeout      = flag.Duration("gate-timeout", 5*time.Minute, "How long a pod may stay gated waiting for the topology of its snapshots before the gate is removed without it. Must be positive.")

	leaderElection              = flag.Bool("leader-election", false, "Enables leader election for the controller. The webhooks are served by every replica.")
	leaderElectionNamespace     = flag.String("leader-election-namespace", "", "The namespace where the leader election resource exists. Defaults to the pod namespace if not set.")
	leaderElectionLeaseDuration = flag.Duration("leader-election-lease-duration", 15*time.Second, "Duration that non-leader candidates will wait to force acquire leadership.")
	leaderElectionRenewDeadline = flag.Duration("leader-election-renew-deadline", 10*time.Second, "Duration that the acting leader will retry refreshing leadership before giving up.")
	leaderElectionRetryPeriod   = flag.Duration("leader-election-retry-period", 5*time.Second, "Duration the LeaderElector clients should wait between tries of actions.")

	kubeAPIQPS         = flag.Float64("kube-api-qps", 20, "QPS to use while communicating with the kubernetes apiserver.")
	kubeAPIBurst       = flag.Int("kube-api-burst", 50, "Burst to use while communicating with the kubernetes apiserver.")
	retryIntervalStart = flag.Duration("retry-interval-start", time.Second, "Initial retry interval for a pod that is waiting for its snapshot topology or failed to sync. It doubles with each failure, up to retry-interval-max.")
	retryIntervalMax   = flag.Duration("retry-interval-max", 5*time.Minute, "Maximum retry interval for a pod that is waiting for its snapshot topology or failed to sync.")

	featureGates map[string]bool
)

var version = "unknown"

func main() {
	flag.Var(utilflag.NewMapStringBool(&featureGates), "feature-gates", "Comma-separated list of key=value pairs that describe feature gates for alpha/experimental features. "+
		"Options are:\n"+strings.Join(utilfeature.DefaultFeatureGate.KnownFeatures(), "\n"))
	fg := featuregate.NewFeatureGate()
	logsapi.AddFeatureGates(fg)
	c := logsapi.NewLoggingConfiguration()
	logsapi.AddGoFlags(c, flag.CommandLine)
	logs.InitLogs()
	standardflags.AddAutomaxprocs(klog.Infof)
	flag.Parse()
	if err := logsapi.ValidateAndApply(c, fg); err != nil {
		klog.ErrorS(err, "LoggingConfiguration is invalid")
		klog.FlushAndExit(klog.ExitFlushTimeout, 1)
	}
	if err := utilfeature.DefaultMutableFeatureGate.SetFromMap(featureGates); err != nil {
		klog.Fatalf("Error while parsing feature gates: %v", err)
	}
	if *showVersion {
		fmt.Println(os.Args[0], version)
		os.Exit(0)
	}
	klog.InfoS("Version", "version", version)
	if *certFile == "" || *keyFile == "" {
		klog.Fatal("--tls-cert-file and --tls-private-key-file must be specified")
	}
	for name, value := range map[string]float64{
		"--max-affinity-terms": float64(*maxAffinityTerms),
		"--worker-threads":     float64(*threads),
		"--gate-timeout":       float64(*gateTimeout),
	} {
		if value <= 0 {
			klog.Fatalf("%s must be positive, got %v", name, value)
		}
	}

	enabled := func() bool { return utilfeature.DefaultFeatureGate.Enabled(features.VolumeSnapshotTopology) }
	if !enabled() {
		klog.Warningf("The %s feature gate is disabled: pods and claims are admitted unchanged and remaining scheduling gates are removed", features.VolumeSnapshotTopology)
	}

	config, err := buildConfig(*kubeconfig)
	if err != nil {
		klog.Fatal(err)
	}
	config.QPS = float32(*kubeAPIQPS)
	config.Burst = *kubeAPIBurst
	coreConfig := rest.CopyConfig(config)
	coreConfig.ContentType = runtime.ContentTypeProtobuf
	kubeClient, err := kubernetes.NewForConfig(coreConfig)
	if err != nil {
		klog.Fatal(err)
	}
	snapClient, err := clientset.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Error building snapshot clientset: %v", err)
	}

	// Delay ctx cancellation until HTTP draining finishes: stopping leader
	// election exits the process and would cut the drain short.
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var shuttingDown atomic.Bool

	coreFactory := informers.NewSharedInformerFactory(kubeClient, *resyncPeriod)
	snapFactory := snapinformers.NewSharedInformerFactory(snapClient, *resyncPeriod)
	pvcInformer := coreFactory.Core().V1().PersistentVolumeClaims()
	scInformer := coreFactory.Storage().V1().StorageClasses()
	nodeInformer := coreFactory.Core().V1().Nodes()
	snapshotInformer := snapFactory.Snapshot().V1().VolumeSnapshots()
	contentInformer := snapFactory.Snapshot().V1().VolumeSnapshotContents()
	synced := []cache.InformerSynced{
		pvcInformer.Informer().HasSynced,
		scInformer.Informer().HasSynced,
		nodeInformer.Informer().HasSynced,
		snapshotInformer.Informer().HasSynced,
		contentInformer.Informer().HasSynced,
	}
	resolver := &topology.Resolver{
		PVCs:           pvcInformer.Lister(),
		StorageClasses: scInformer.Lister(),
		Snapshots:      snapshotInformer.Lister(),
		Contents:       contentInformer.Lister(),
		LiveClaims: func(ctx context.Context, namespace, name string) (*v1.PersistentVolumeClaim, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
		},
	}
	coreFactory.Start(ctx.Done())
	snapFactory.Start(ctx.Done())

	wh := &topologywebhook.Webhook{
		Resolver:         resolver,
		Nodes:            nodeInformer.Lister(),
		MaxAffinityTerms: *maxAffinityTerms,
		Enabled:          enabled,
		Synced: func() bool {
			if shuttingDown.Load() {
				return false
			}
			for _, s := range synced {
				if !s() {
					return false
				}
			}
			return true
		},
	}
	srv, err := newServer(ctx, wh.Handler())
	if err != nil {
		klog.Fatalf("Failed to start webhook server: %v", err)
	}
	go func() {
		if err := srv.serve(); err != nil && err != http.ErrServerClosed {
			klog.Fatalf("Webhook server stopped: %v", err)
		}
	}()
	go func() {
		<-sigCtx.Done()
		klog.Infof("Received termination signal, reporting unready for %v before shutting down", *shutdownDelay)
		shuttingDown.Store(true)
		time.Sleep(*shutdownDelay)
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			klog.Errorf("Webhook server shutdown: %v", err)
		}
		cancel()
	}()

	// Only the leader watches pods.
	run := func(ctx context.Context) {
		podFactory := informers.NewSharedInformerFactoryWithOptions(kubeClient, *resyncPeriod,
			informers.WithTweakListOptions(topologycontroller.PodListOptions))
		ctrl, err := topologycontroller.NewController(topologycontroller.Config{
			Enabled:            enabled,
			MaxAffinityTerms:   *maxAffinityTerms,
			GateTimeout:        *gateTimeout,
			RetryIntervalStart: *retryIntervalStart,
			RetryIntervalMax:   *retryIntervalMax,
			ResolverSynced:     synced,
		}, kubeClient, resolver, podFactory.Core().V1().Pods(), pvcInformer, snapshotInformer, contentInformer)
		if err != nil {
			klog.Fatalf("Failed to create controller: %v", err)
		}
		podFactory.Start(ctx.Done())
		ctrl.Run(ctx, *threads)
	}

	if !*leaderElection {
		run(ctx)
		return
	}
	// Keep controller throttling from delaying lease renewals.
	leClientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("Failed to create leader election client: %v", err)
	}
	le := leaderelection.NewLeaderElection(leClientset, "snapshot-topology-controller-leader", run)
	if *leaderElectionNamespace != "" {
		le.WithNamespace(*leaderElectionNamespace)
	}
	le.WithLeaseDuration(*leaderElectionLeaseDuration)
	le.WithRenewDeadline(*leaderElectionRenewDeadline)
	le.WithRetryPeriod(*leaderElectionRetryPeriod)
	le.WithContext(ctx)
	le.WithReleaseOnCancel(true)
	if err := le.Run(); err != nil {
		klog.Fatalf("Failed to initialize leader election: %v", err)
	}
}

type server struct {
	*http.Server
	listener net.Listener
}

func newServer(ctx context.Context, handler http.Handler) (*server, error) {
	cw, err := webhook.NewCertWatcher(*certFile, *keyFile)
	if err != nil {
		return nil, fmt.Errorf("initializing cert watcher: %w", err)
	}
	go func() {
		if err := cw.Start(ctx); err != nil {
			klog.Errorf("Certificate watcher error: %v", err)
		}
	}()
	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", *port), &tls.Config{GetCertificate: cw.GetCertificate})
	if err != nil {
		return nil, err
	}
	return &server{
		Server: &http.Server{
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  15 * time.Second,
		},
		listener: listener,
	}, nil
}

func (s *server) serve() error {
	return s.Serve(s.listener)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
