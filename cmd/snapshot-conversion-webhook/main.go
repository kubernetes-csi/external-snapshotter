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

package main

import (
	"context"
	"crypto/tls"
	"flag"

	"k8s.io/component-base/logs"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/klog/v2"

	"github.com/kubernetes-csi/csi-lib-utils/standardflags"
	"github.com/kubernetes-csi/external-snapshotter/v8/pkg/webhook"
)

var (
	certFile = flag.String(
		"tls-cert-file",
		"",
		"File containing the x509 Certificate for HTTPS. (CA cert, if any, concatenated after server cert). Required.",
	)
	keyFile = flag.String(
		"tls-private-key-file",
		"",
		"File containing the x509 private key matching --tls-cert-file. Required.",
	)
	port = flag.Int(
		"port",
		443,
		"Secure port that the webhook listens on",
	)
)

func main() {
	c := logsapi.NewLoggingConfiguration()
	logsapi.AddGoFlags(c, flag.CommandLine)
	logs.InitLogs()
	standardflags.AddAutomaxprocs(klog.Infof)
	flag.Parse()

	klog.Info("Starting conversion webhook server")

	if certFile == nil || *certFile == "" {
		klog.Fatal("--tls-cert-file must be specified")
	}
	if keyFile == nil || *keyFile == "" {
		klog.Fatal("--tls-private-key-file must be specified")
	}

	// Create new cert watcher
	cw, err := webhook.NewCertWatcher(*certFile, *keyFile)
	if err != nil {
		klog.Fatalf("failed to initialize new cert watcher: %v", err)
	}
	tlsConfig := &tls.Config{
		GetCertificate: cw.GetCertificate,
	}

	// Start the webhook server
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // stops certwatcher

	if err := webhook.StartServer(ctx, tlsConfig, cw, *port); err != nil {
		klog.Fatalf("server stopped: %v", err)
	}
}
