/*
Copyright 2017 The Kubernetes Authors.

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

package utils

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// Connect address by grpc
func Connect(address string, dialOptions ...grpc.DialOption) (*grpc.ClientConn, error) {
	u, err := url.Parse(address)
	if err == nil && (!u.IsAbs() || u.Scheme == "unix") {
		dialOptions = append(dialOptions,
			grpc.WithContextDialer(
				func(ctx context.Context, addr string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", u.Path)
				}))
		if u.Scheme != "unix" {
			address = "unix:" + address
		}
	}

	conn, err := grpc.NewClient(address, dialOptions...)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return conn, nil
		}
		if !conn.WaitForStateChange(ctx, state) {
			return conn, fmt.Errorf("Connection timed out")
		}
	}
}
