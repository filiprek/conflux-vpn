//go:build !windows

package control

import (
	"context"
	"errors"
)

func CallPipe(ctx context.Context, name string, q Request) (Response, error) {
	return Response{}, errors.New("conflux desktop control requires Windows")
}
