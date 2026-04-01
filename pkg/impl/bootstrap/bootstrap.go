// Package bootstrap ensures all source and resolver implementations are registered.
// Import this package to automatically register all implementations:
//
//	import _ "github.com/ystkfujii/tring/pkg/impl/bootstrap"
package bootstrap

import (
	// Import all source implementations to trigger their init() functions
	_ "github.com/ystkfujii/tring/pkg/impl/sources/aqua"
	_ "github.com/ystkfujii/tring/pkg/impl/sources/envfile"
	_ "github.com/ystkfujii/tring/pkg/impl/sources/githubaction"
	_ "github.com/ystkfujii/tring/pkg/impl/sources/gomod"

	// Import all resolver implementations to trigger their init() functions
	_ "github.com/ystkfujii/tring/pkg/impl/resolver/aqua_registry"
	_ "github.com/ystkfujii/tring/pkg/impl/resolver/githubrelease"
	_ "github.com/ystkfujii/tring/pkg/impl/resolver/goproxy"
	_ "github.com/ystkfujii/tring/pkg/impl/resolver/gotoolchain"
)
