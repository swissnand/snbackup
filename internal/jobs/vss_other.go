//go:build !windows

package jobs

import (
	"context"
	"errors"
)

func createShadows(ctx context.Context, paths []string, log func(level, msg string)) (func(string) string, func(), error) {
	return nil, nil, errors.New("VSS not available on this platform")
}
