// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package logs // import "github.com/hanzoai/otel-collector/processor/o11ytransformprocessor/internal/logs"

import (
	"fmt"

	o11yFuncs "github.com/hanzoai/otel-collector/processor/o11ytransformprocessor/ottlfunctions"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/contexts/ottllog"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/ottl/ottlfuncs"
)

func O11yLogFunctions() map[string]ottl.Factory[*ottllog.TransformContext] {
	factoryMap := map[string]ottl.Factory[*ottllog.TransformContext]{}
	for _, f := range []ottl.Factory[*ottllog.TransformContext]{
		o11yFuncs.NewExprFactory(),
		o11yFuncs.NewGrokParseFactory[*ottllog.TransformContext](),
		o11yFuncs.NewHexToIntFactory[*ottllog.TransformContext](),
	} {
		factoryMap[f.Name()] = f
	}
	return factoryMap
}

func LogFunctions() map[string]ottl.Factory[*ottllog.TransformContext] {
	logFunctions := ottlfuncs.StandardFuncs[*ottllog.TransformContext]()

	for name, factory := range O11yLogFunctions() {
		_, exists := logFunctions[name]
		if exists {
			panic(fmt.Sprintf("ottl func %s already exists", name))
		}
		logFunctions[name] = factory
	}

	return logFunctions
}
