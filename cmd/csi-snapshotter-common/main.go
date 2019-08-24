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
	"strings"
	"time"

	"google.golang.org/grpc"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/leaderelection"
	csirpc "github.com/kubernetes-csi/csi-lib-utils/rpc"
	controller "github.com/kubernetes-csi/external-snapshotter/pkg/common_controller"

	clientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	snapshotscheme "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned/scheme"
	informers "github.com/kubernetes-csi/external-snapshotter/pkg/client/informers/externalversions"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	coreinformers "k8s.io/client-go/informers"
)

const (
	// Number of worker threads
	threads = 10
)

// Command line flags
var (
	kubeconfig   = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	resyncPeriod = flag.Duration("resync-period", 60*time.Second, "Resync interval of the controller.")
	showVersion  = flag.Bool("version", false, "Show version.")

	leaderElection          = flag.Bool("leader-election", false, "Enables leader election.")
	leaderElectionNamespace = flag.String("leader-election-namespace", "", "The namespace where the leader election resource exists. Defaults to the pod namespace if not set.")
)

var (
	version = "unknown"
	prefix  = "external-snapshotter-leader"
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

	// Create CRD resource
	aeclientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// initialize CRD resource if it does not exist
	err = CreateCRD(aeclientset)
	if err != nil {
		klog.Error(err.Error())
		os.Exit(1)
	}

	// Add Snapshot types to the defualt Kubernetes so events can be logged for them
	snapshotscheme.AddToScheme(scheme.Scheme)

	klog.V(2).Infof("Start NewCSISnapshotController with kubeconfig [%s] resyncPeriod [%+v]", *kubeconfig, *resyncPeriod)

	ctrl := controller.NewCSISnapshotCommonController(
		snapClient,
		kubeClient,
		factory.Snapshot().V1alpha1().VolumeSnapshots(),
		factory.Snapshot().V1alpha1().VolumeSnapshotContents(),
		factory.Snapshot().V1alpha1().VolumeSnapshotClasses(),
		coreFactory.Core().V1().PersistentVolumeClaims(),
		*resyncPeriod,
	)

	run := func(context.Context) {
		// run...
		stopCh := make(chan struct{})
		factory.Start(stopCh)
		coreFactory.Start(stopCh)
		go ctrl.Run(threads, stopCh)

		// ...until SIGINT
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		close(stopCh)
	}

	if !*leaderElection {
		run(context.TODO())
	} else {
		lockName := fmt.Sprintf("%s-%s", prefix, strings.Replace("common-snapshotter", "/", "-", -1))
		le := leaderelection.NewLeaderElection(kubeClient, lockName, run)
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

func supportsControllerCreateSnapshot(ctx context.Context, conn *grpc.ClientConn) (bool, error) {
	capabilities, err := csirpc.GetControllerCapabilities(ctx, conn)
	if err != nil {
		return false, err
	}

	return capabilities[csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT], nil
}
