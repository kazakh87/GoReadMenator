package cli

import (
	"context"

	"goreadmenator/internal/gitx"
)

func gitHead(ctx context.Context, dir string) (string, error) {
	return gitx.Runner{Dir: dir}.Head(ctx)
}
