//go:build !lunar_private

// Package lunar retains type compatibility for existing private profiles.
// Official PixelPlay builds do not include or launch Lunar.
package lunar

import (
	"context"
	"errors"
)

var Enabled = ""

var ErrNotAvailable = errors.New("Lunar no está disponible en la versión pública; usa Horizon o un loader Java")

type RuntimeVersion struct {
	Java          int
	ContentLoader string
}

func RuntimeForVersion(string) (RuntimeVersion, error) {
	return RuntimeVersion{}, ErrNotAvailable
}

type Options struct {
	Version, GameDir, AssetsDir, AssetIndex, Username, UUID, AccessToken, ClientJar, Expires string
	Memory                                                                                   int
	Guest                                                                                    bool
}

func Prepare(context.Context, Options, func(string)) ([]string, error) {
	return nil, ErrNotAvailable
}
