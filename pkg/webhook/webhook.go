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

package webhook

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	"k8s.io/klog/v2"
)

func StartServer(
	ctx context.Context,
	tlsConfig *tls.Config,
	cw *CertWatcher,
	port int,
) error {
	go func() {
		klog.Info("Starting certificate watcher")
		if err := cw.Start(ctx); err != nil {
			klog.Errorf("certificate watcher error: %v", err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/convert", func(w http.ResponseWriter, req *http.Request) { serve(w, req, convertGroupSnapshotCRD) })

	srv := &http.Server{
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	// listener is always closed by srv.Serve
	listener, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), tlsConfig)
	if err != nil {
		return err
	}

	return srv.Serve(listener)
}
