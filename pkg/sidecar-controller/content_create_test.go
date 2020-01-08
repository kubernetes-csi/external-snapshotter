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

package sidecar_controller

import (
	"testing"
	"time"
)

func TestSyncContent(t *testing.T) {
	var tests []controllerTest

	tests = append(tests, controllerTest{
		name:             "Basic content update ready to use",
		initialContents:  newContentArrayWithReadyToUse("content1-1", "snapuid1-1", "snap1-1", "sid1-1", defaultClass, "", "volume-handle-1-1", retainPolicy, nil, &defaultSize, &False, true),
		expectedContents: newContentArrayWithReadyToUse("content1-1", "snapuid1-1", "snap1-1", "sid1-1", defaultClass, "", "volume-handle-1-1", retainPolicy, nil, &defaultSize, &True, true),
		expectedEvents:   noevents,
		expectedCreateCalls: []createCall{
			{
				volumeHandle: "volume-handle-1-1",
				snapshotName: "snapshot-snapuid1-1",
				driverName:   mockDriverName,
				snapshotId:   "snapuid1-1",
				creationTime: timeNow,
				readyToUse:   true,
			},
		},
		expectedListCalls: []listCall{{"sid1-1", true, time.Now(), 1, nil}},
		errors:            noerrors,
		test:              testSyncContent,
	})

	runSyncContentTests(t, tests, snapshotClasses)
}
