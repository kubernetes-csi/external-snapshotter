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
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"
)

// This file originated from github.com/kubernetes-sigs/controller-runtime/pkg/webhook/internal/certwatcher.
// We cannot import this package as it's an internal one. In addition, we cannot yet easily integrate
// with controller-runtime/pkg/webhook directly, as it would require extensive rework:
// https://github.com/kubernetes-csi/external-snapshotter/issues/422

// CertWatcher watches certificate and key files for changes.  When either file
// changes, it reads and parses both and calls an optional callback with the new
// certificate.
type CertWatcher struct {
	// We will have too many reads and fewer writes
	// RWMutex will help avoid the serialization bottleneck
	sync.RWMutex

	currentCert *tls.Certificate
	watcher     *fsnotify.Watcher

	certPath string
	keyPath  string
}

// NewCertWatcher returns a new CertWatcher watching the given certificate and key.
func NewCertWatcher(certPath, keyPath string) (*CertWatcher, error) {
	var err error

	cw := &CertWatcher{
		certPath: certPath,
		keyPath:  keyPath,
	}

	// Initial read of certificate and key.
	if err := cw.readCertificate(); err != nil {
		return nil, err
	}

	cw.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return cw, nil
}

// GetCertificate fetches the currently loaded certificate from the memory, it might be nil
// if called before readCertificate.
func (cw *CertWatcher) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cw.RLock()
	defer cw.RUnlock()
	return cw.currentCert, nil
}

// Start starts the watch on the certificate and key files.
func (cw *CertWatcher) Start(ctx context.Context) error {
	// Many editors write to a temp dir and then rename the file
	// watch the directory instead to remediate
	files := []string{filepath.Dir(cw.certPath)}

	// Might be possible that the cert key is in a different dir
	if keyDir := filepath.Dir(cw.keyPath); keyDir != files[0] {
		files = append(files, keyDir)
	}

	for _, f := range files {
		if err := cw.watcher.Add(f); err != nil {
			return err
		}
	}

	go cw.Watch()

	// Block until the context is done.
	<-ctx.Done()

	return cw.watcher.Close()
}

// Watch reads events from the watcher's channel and reacts to changes.
func (cw *CertWatcher) Watch() {
	for {
		select {
		case event, ok := <-cw.watcher.Events:
			// Channel is closed.
			if !ok {
				return
			}

			cw.handleEvent(event)

		case err, ok := <-cw.watcher.Errors:
			// Channel is closed.
			if !ok {
				return
			}

			klog.Error(err, "certificate watch error")
		}
	}
}

// readCertificate reads the certificate and key files from disk, parses them,
// and updates the current certificate on the watcher.  If a callback is set, it
// is invoked with the new certificate.
func (cw *CertWatcher) readCertificate() error {
	cert, err := tls.LoadX509KeyPair(cw.certPath, cw.keyPath)
	if err != nil {
		return err
	}

	cw.Lock()
	cw.currentCert = &cert
	cw.Unlock()

	klog.Info("Updated current TLS certificate")

	return nil
}

func (cw *CertWatcher) handleEvent(event fsnotify.Event) {
	// Return if the event is for neither cert nor key
	if event.Name != cw.certPath && event.Name != cw.keyPath {
		return
	}

	// If event is not write, create or rename, return
	if !(event.Op&fsnotify.Write == fsnotify.Write ||
		event.Op&fsnotify.Create == fsnotify.Create ||
		event.Op&fsnotify.Rename == fsnotify.Rename) { // Important for atomic write eg. vi/similar
		return
	}

	klog.V(1).Info("certificate event", "event", event)

	// Re-read and update our copy of the certificate
	if err := cw.readCertificate(); err != nil {
		klog.Error(err, "error re-reading certificate")
	}
}
