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

// Package standardflags contains flags that multiple CSI sidecars and
// drivers may want to support.
package standardflags

import (
	"flag"
	"fmt"
	"strconv"

	"go.uber.org/automaxprocs/maxprocs"
)

var (
	logFunc          func(format string, args ...interface{}) = nil
	undoAutomaxprocs func()                                   = nil
)

// AddAutomaxprocs adds the -automaxprocs boolean flag to the commandline options.
// By default the flag is disabled, use [EnableAutomaxprocs] to enable it during
// startup of an application.
//
// The printf function that is passed as an argument, will be used by the
// [maxprocs.Logger] option when the GOMAXPROCS runtime configuration is adjusted.
func AddAutomaxprocs(printf func(format string, args ...interface{})) {
	flag.BoolFunc("automaxprocs",
		"automatically set GOMAXPROCS to match Linux container CPU quota",
		handleAutomaxprocs,
	)

	if printf != nil {
		// maxprocs.Logger expects a Printf like function.
		// klog.Info() isn't one, so wrap the contents in a
		// fmt.Sprintf() for %-formatting substitution.
		logFunc = func(f string, a ...interface{}) {
			printf(fmt.Sprintf(f, a...))
		}
	}
}

// EnableAutomaxprocs can be used as an equivalent of -automaxprocs=true on the
// commandline.
func EnableAutomaxprocs() {
	if automaxprocsIsEnabled() {
		// enabled already, don't enable again
		return
	}

	flag.Set("automaxprocs", "true")
}

// automaxprocsIsEnabled returns true if maxprocs.Set() was successfully
// executed.
func automaxprocsIsEnabled() bool {
	return undoAutomaxprocs != nil
}

// handleAutomaxprocs parses the passed string into a bool, and enables
// automaxprocs according to it. If the passed string is empty, automaxprocs
// is enabled as well.
func handleAutomaxprocs(s string) error {
	var err error
	enabled := true

	if s == "" {
		EnableAutomaxprocs()
		return nil
	}

	enabled, err = strconv.ParseBool(s)
	if err != nil {
		return err
	}

	switch enabled {
	case true:
		opts := make([]maxprocs.Option, 0)
		if logFunc != nil {
			opts = append(opts, maxprocs.Logger(logFunc))
		}
		undoAutomaxprocs, err = maxprocs.Set(opts...)
	case false:
		if undoAutomaxprocs != nil {
			undoAutomaxprocs()
			undoAutomaxprocs = nil
		}
	}

	return err
}
