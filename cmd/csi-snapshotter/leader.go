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
	"fmt"
	"os"
	"time"

	"github.com/golang/glog"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
)

const (
	leaseDuration = 15 * time.Second
	renewDeadline = 10 * time.Second
	retryPeriod   = 5 * time.Second
)

// WaitForLeader waits until this particular external snapshotter becomes a leader.
func WaitForLeader(clientset *kubernetes.Clientset, namespace string, lockName string) {
	hostname, err := os.Hostname()
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}
	// add a uniquifier so that two processes on the same host don't accidentally both become active
	identity := hostname + "_" + string(uuid.NewUUID())

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: clientset.CoreV1().Events(namespace)})
	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("%s %s", lockName, string(identity))})

	rlConfig := resourcelock.ResourceLockConfig{
		Identity:      identity,
		EventRecorder: eventRecorder,
	}
	lock, err := resourcelock.New(resourcelock.ConfigMapsResourceLock, namespace, lockName, clientset.CoreV1(), rlConfig)
	if err != nil {
		glog.Error(err)
		os.Exit(1)
	}

	elected := make(chan struct{})

	leaderConfig := leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: leaseDuration,
		RenewDeadline: renewDeadline,
		RetryPeriod:   retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(stop <-chan struct{}) {
				glog.V(2).Info("Became leader, starting")
				close(elected)
			},
			OnStoppedLeading: func() {
				glog.Error("Stopped leading")
				os.Exit(1)
			},
			OnNewLeader: func(identity string) {
				glog.V(3).Infof("Current leader: %s", identity)
			},
		},
	}

	go leaderelection.RunOrDie(leaderConfig)

	// wait for being elected
	<-elected
}
