package canvas

import "errors"

// ErrViewName is returned by View.Validate when a view ID does not conform
// to the stable format (lowercase kebab-case ASCII, non-empty).
var ErrViewName = errors.New("canvas: invalid view ID")

// ErrUnknownView is returned when a view ID is not registered under
// ContractVersion. Callers gate on it with errors.Is to distinguish a
// contract-drift view name from other failures.
var ErrUnknownView = errors.New("canvas: unknown view")

// ErrAssetSource is returned by NewRenderer when the caller supplies a nil
// AssetSource. A Renderer cannot serve view documents without one: the
// composition root (the CLI's embedded FS or the hosted product's artifact
// store) owns asset delivery, and the canvas module embeds no bundles of its
// own to fall back to.
var ErrAssetSource = errors.New("canvas: missing asset source")

// ErrMissingPayload is returned by View.Payload for a registered view that
// has no server-provided payload contract. Such views bootstrap themselves
// by invoking their actions (Actions) instead of being seeded with initial
// data; the server passes no payload into their document today.
var ErrMissingPayload = errors.New("canvas: view has no typed payload")
