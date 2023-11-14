/*
Copyright 2019 The Kubernetes Authors.

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

package leaderelection

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

const (
	defaultLeaseDuration = 15 * time.Second
	defaultRenewDeadline = 10 * time.Second
	defaultRetryPeriod   = 5 * time.Second

	DefaultHealthCheckTimeout = 20 * time.Second

	// HealthCheckerAddress is the address at which the leader election health
	// checker reports status.
	// The caller sidecar should document this address in appropriate flag
	// descriptions.
	HealthCheckerAddress = "/healthz/leader-election"
)

// leaderElection is a convenience wrapper around client-go's leader election library.
type leaderElection struct {
	runFunc func(ctx context.Context)

	// the lockName identifies the leader election config and should be shared across all members
	lockName string
	// the identity is the unique identity of the currently running member
	identity string
	// the namespace to store the lock resource
	namespace string
	// resourceLock defines the type of leaderelection that should be used
	// Only resourcelock.LeasesResourceLock is valid at the moment.
	resourceLock string
	// healthCheck reports unhealthy if leader election fails to renew leadership
	// within a timeout period.
	healthCheck *leaderelection.HealthzAdaptor

	leaseDuration time.Duration
	renewDeadline time.Duration
	retryPeriod   time.Duration

	ctx context.Context

	clientset kubernetes.Interface
}

// NewLeaderElection returns the default & preferred leader election type
func NewLeaderElection(clientset kubernetes.Interface, lockName string, runFunc func(ctx context.Context)) *leaderElection {
	return NewLeaderElectionWithLeases(clientset, lockName, runFunc)
}

// NewLeaderElectionWithLeases returns an implementation of leader election using Leases
func NewLeaderElectionWithLeases(clientset kubernetes.Interface, lockName string, runFunc func(ctx context.Context)) *leaderElection {
	return &leaderElection{
		runFunc:       runFunc,
		lockName:      lockName,
		resourceLock:  resourcelock.LeasesResourceLock,
		leaseDuration: defaultLeaseDuration,
		renewDeadline: defaultRenewDeadline,
		retryPeriod:   defaultRetryPeriod,
		clientset:     clientset,
	}
}

func (l *leaderElection) WithIdentity(identity string) {
	l.identity = identity
}

func (l *leaderElection) WithNamespace(namespace string) {
	l.namespace = namespace
}

func (l *leaderElection) WithLeaseDuration(leaseDuration time.Duration) {
	l.leaseDuration = leaseDuration
}

func (l *leaderElection) WithRenewDeadline(renewDeadline time.Duration) {
	l.renewDeadline = renewDeadline
}

func (l *leaderElection) WithRetryPeriod(retryPeriod time.Duration) {
	l.retryPeriod = retryPeriod
}

// WithContext Add context
func (l *leaderElection) WithContext(ctx context.Context) {
	l.ctx = ctx
}

// Server represents any type that could serve HTTP requests for the leader
// election health check endpoint.
type Server interface {
	Handle(pattern string, handler http.Handler)
}

// PrepareHealthCheck creates a health check for this leader election object
// with the given healthCheckTimeout and registers its HTTP handler to the given
// server at the path specified by the constant "healthCheckerAddress".
// healthCheckTimeout determines the max duration beyond lease expiration
// allowed before reporting unhealthy.
// The caller sidecar should document the handler address in appropriate flag
// descriptions.
func (l *leaderElection) PrepareHealthCheck(
	s Server,
	healthCheckTimeout time.Duration) {

	l.healthCheck = leaderelection.NewLeaderHealthzAdaptor(healthCheckTimeout)
	s.Handle(HealthCheckerAddress, adaptCheckToHandler(l.healthCheck.Check))
}

func (l *leaderElection) Run() error {
	if l.identity == "" {
		id, err := defaultLeaderElectionIdentity()
		if err != nil {
			return fmt.Errorf("error getting the default leader identity: %v", err)
		}

		l.identity = id
	}

	if l.namespace == "" {
		l.namespace = inClusterNamespace()
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: l.clientset.CoreV1().Events(l.namespace)})
	eventRecorder := broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: fmt.Sprintf("%s/%s", l.lockName, string(l.identity))})

	rlConfig := resourcelock.ResourceLockConfig{
		Identity:      sanitizeName(l.identity),
		EventRecorder: eventRecorder,
	}

	lock, err := resourcelock.New(l.resourceLock, l.namespace, sanitizeName(l.lockName), l.clientset.CoreV1(), l.clientset.CoordinationV1(), rlConfig)
	if err != nil {
		return err
	}

	leaderConfig := leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: l.leaseDuration,
		RenewDeadline: l.renewDeadline,
		RetryPeriod:   l.retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				klog.V(2).Info("became leader, starting")
				l.runFunc(ctx)
			},
			OnStoppedLeading: func() {
				klog.Fatal("stopped leading")
			},
			OnNewLeader: func(identity string) {
				klog.V(3).Infof("new leader detected, current leader: %s", identity)
			},
		},
		WatchDog: l.healthCheck,
	}

	ctx := l.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	leaderelection.RunOrDie(ctx, leaderConfig)
	return nil // should never reach here
}

func defaultLeaderElectionIdentity() (string, error) {
	return os.Hostname()
}

// sanitizeName sanitizes the provided string so it can be consumed by leader election library
func sanitizeName(name string) string {
	re := regexp.MustCompile("[^a-zA-Z0-9-]")
	name = re.ReplaceAllString(name, "-")
	if name[len(name)-1] == '-' {
		// name must not end with '-'
		name = name + "X"
	}
	return name
}

// inClusterNamespace returns the namespace in which the pod is running in by checking
// the env var POD_NAMESPACE, then the file /var/run/secrets/kubernetes.io/serviceaccount/namespace.
// if neither returns a valid namespace, the "default" namespace is returned
func inClusterNamespace() string {
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	if data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		if ns := strings.TrimSpace(string(data)); len(ns) > 0 {
			return ns
		}
	}

	return "default"
}

// adaptCheckToHandler returns an http.HandlerFunc that serves the provided checks.
func adaptCheckToHandler(c func(r *http.Request) error) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := c(r)
		if err != nil {
			http.Error(w, fmt.Sprintf("internal server error: %v", err), http.StatusInternalServerError)
		} else {
			fmt.Fprint(w, "ok")
		}
	})
}
