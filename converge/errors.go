package converge

import "errors"

// ErrNilCatalog is returned by RegisterAll when called with a nil catalog.
var ErrNilCatalog = errors.New("converge: nil opmesh catalog")
