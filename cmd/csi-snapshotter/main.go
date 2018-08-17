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
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubernetes-csi/external-snapshotter/pkg/connection"
	"github.com/kubernetes-csi/external-snapshotter/pkg/controller"

	clientset "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	informers "github.com/kubernetes-csi/external-snapshotter/pkg/client/informers/externalversions"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

const (
	// Number of worker threads
	threads = 10

	// Default timeout of short CSI calls like GetPluginInfo
	csiTimeout = time.Second
)

// Command line flags
var (
	snapshotter                     = flag.String("snapshotter", "", "Name of the snapshotter. The snapshotter will only create snapshot content for snapshot that requests a VolumeSnapshotClass with a snapshotter field set equal to this name.")
	kubeconfig                      = flag.String("kubeconfig", "", "Absolute path to the kubeconfig file. Required only when running out of cluster.")
	connectionTimeout               = flag.Duration("connection-timeout", 1*time.Minute, "Timeout for waiting for CSI driver socket.")
	csiAddress                      = flag.String("csi-address", "/run/csi/socket", "Address of the CSI driver socket.")
	createSnapshotContentRetryCount = flag.Int("create-snapshotcontent-retrycount", 5, "Number of retries when we create a snapshot content object for a snapshot.")
	createSnapshotContentInterval   = flag.Duration("create-snapshotcontent-interval", 10*time.Second, "Interval between retries when we create a snapshot content object for a snapshot.")
	resyncPeriod                    = flag.Duration("resync-period", 60*time.Second, "Resync interval of the controller.")
	snapshotNamePrefix              = flag.String("snapshot-name-prefix", "snapshot", "Prefix to apply to the name of a created snapshot")
	snapshotNameUUIDLength          = flag.Int("snapshot-name-uuid-length", -1, "Length in characters for the generated uuid of a created snapshot. Defaults behavior is to NOT truncate.")
)

func main() {
	flag.Set("logtostderr", "true")
	flag.Parse()

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}

	snapClient, err := clientset.NewForConfig(config)
	if err != nil {
		glog.Errorf("Error building snapshot clientset: %s", err.Error())
		os.Exit(1)
	}

	factory := informers.NewSharedInformerFactory(snapClient, *resyncPeriod)

	// Create CRD resource
	aeclientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}

	// initialize CRD resource if it does not exist
	err = CreateCRD(aeclientset)
	if err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}

	// Connect to CSI.
	csiConn, err := connection.New(*csiAddress, *connectionTimeout)
	if err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}

	// Pass a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
	defer cancel()

	// Find driver name
	if *snapshotter == "" {
		*snapshotter, err = csiConn.GetDriverName(ctx)
		if err != nil {
			glog.Error(err.Error())
			os.Exit(1)
		}
	}
	glog.V(2).Infof("CSI driver name: %q", *snapshotter)

	// Check it's ready
	if err = waitForDriverReady(csiConn, *connectionTimeout); err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}

	// Find out if the driver supports create/delete snapshot.
	supportsCreateSnapshot, err := csiConn.SupportsControllerCreateSnapshot(ctx)
	if err != nil {
		glog.Error(err.Error())
		os.Exit(1)
	}
	if !supportsCreateSnapshot {
		glog.Errorf("CSI driver %s does not support ControllerCreateSnapshot", *snapshotter)
		os.Exit(1)
	}

	if len(*snapshotNamePrefix) == 0 {
		glog.Error("Snapshot name prefix cannot be of length 0")
		os.Exit(1)
	}

	glog.V(2).Infof("Start NewCSISnapshotController with snapshotter [%s] kubeconfig [%s] connectionTimeout [%+v] csiAddress [%s] createSnapshotContentRetryCount [%d] createSnapshotContentInterval [%+v] resyncPeriod [%+v] snapshotNamePrefix [%s] snapshotNameUUIDLength [%d]", *snapshotter, *kubeconfig, *connectionTimeout, *csiAddress, createSnapshotContentRetryCount, *createSnapshotContentInterval, *resyncPeriod, *snapshotNamePrefix, snapshotNameUUIDLength)

	ctrl := controller.NewCSISnapshotController(
		snapClient,
		kubeClient,
		*snapshotter,
		factory.Volumesnapshot().V1alpha1().VolumeSnapshots(),
		factory.Volumesnapshot().V1alpha1().VolumeSnapshotContents(),
		factory.Volumesnapshot().V1alpha1().VolumeSnapshotClasses(),
		*createSnapshotContentRetryCount,
		*createSnapshotContentInterval,
		csiConn,
		*connectionTimeout,
		*resyncPeriod,
		*snapshotNamePrefix,
		*snapshotNameUUIDLength,
	)

	// run...
	stopCh := make(chan struct{})
	factory.Start(stopCh)
	go ctrl.Run(threads, stopCh)

	// ...until SIGINT
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	close(stopCh)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func waitForDriverReady(csiConn connection.CSIConnection, timeout time.Duration) error {
	now := time.Now()
	finish := now.Add(timeout)
	var err error
	for {
		ctx, cancel := context.WithTimeout(context.Background(), csiTimeout)
		defer cancel()
		err = csiConn.Probe(ctx)
		if err == nil {
			glog.V(2).Infof("Probe succeeded")
			return nil
		}
		glog.V(2).Infof("Probe failed with %s", err)

		now := time.Now()
		if now.After(finish) {
			return fmt.Errorf("failed to probe the controller: %s", err)
		}
		time.Sleep(time.Second)
	}
}
