package twmap

import (
	"errors"
	"fmt"
	"io"
)

// ── map validation errors ────────────────────────────────────────────────────

var (
	ErrMissingVersion                  = errors.New("missing map version item")
	ErrInvalidVersion                  = errors.New("invalid map version")
	ErrMissingInfo                     = errors.New("missing map info item")
	ErrNoGameLayer                     = errors.New("no game layer found")
	ErrTooManyGameGroups               = errors.New("more than one group contains game layers")
	ErrTooManyGameLayers               = errors.New("duplicate game/special layer type")
	ErrInconsistentGameLayerDimensions = errors.New("game layer dimensions are inconsistent across special layers")
)

// Validate reads a Teeworlds/DDNet map from r and returns a non-nil error
// if the map is structurally invalid. It checks:
//   - Datafile container integrity (magic, header, items, compressed data)
//   - Map version == 1
//   - Presence of an info item with sufficient fields
//   - All groups and layers parse successfully
//   - All images parse successfully
//   - Exactly one game layer exists
//   - All DDNet special layers (teleport, speedup, front, switch, tune)
//     share the same dimensions as the game layer
func Validate(r io.Reader) error {
	m, err := Parse(r)
	if err != nil {
		return err
	}
	return validateMapStructure(m)
}

func validateMapStructure(m *Map) error {
	var gameWidth, gameHeight int
	gameGroupIdx := -1
	foundGame := false
	foundTele := false
	foundSpeedup := false
	foundFront := false
	foundSwitch := false
	foundTune := false

	for gi, g := range m.Groups {
		for _, l := range g.Layers {
			if l.Kind == LayerKindGame {
				if foundGame {
					return ErrTooManyGameLayers
				}
				foundGame = true
				gameWidth = l.Width
				gameHeight = l.Height
				gameGroupIdx = gi
			}

			type specialCheck struct {
				kind  LayerKind
				found *bool
				name  string
			}
			for _, sc := range []specialCheck{
				{LayerKindTele, &foundTele, "teleport"},
				{LayerKindSpeedup, &foundSpeedup, "speedup"},
				{LayerKindFront, &foundFront, "front"},
				{LayerKindSwitch, &foundSwitch, "switch"},
				{LayerKindTune, &foundTune, "tune"},
			} {
				if l.Kind == sc.kind {
					if *sc.found {
						return fmt.Errorf("%w: duplicate %s layer", ErrTooManyGameLayers, sc.name)
					}
					*sc.found = true

					if gameGroupIdx >= 0 && gameGroupIdx != gi {
						return ErrTooManyGameGroups
					}
					if gameWidth > 0 && (l.Width != gameWidth || l.Height != gameHeight) {
						return ErrInconsistentGameLayerDimensions
					}
				}
			}
		}
	}

	if !foundGame {
		return ErrNoGameLayer
	}
	return nil
}
