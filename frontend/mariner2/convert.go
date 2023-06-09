package mariner2

import (
	"context"

	"github.com/azure/dalec/frontend"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/image"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

const imgRef = "mcr.microsoft.com/cbl-mariner/base/core:2.0"

func Convert(ctx context.Context, spec *frontend.Spec) (llb.State, *image.Image, error) {
	base := llb.Image(imgRef).
		Run(llb.Args([]string{
			"tdnf", "install", "-y", "build-essential", "rpmdevtools",
		})).State

	depsCmd := []string{"tdnf", "install", "-y"}
	for _, dep := range spec.Dependendencies.Build {
		depsCmd = append(depsCmd, dep)
	}
	base = base.Run(llb.Args(depsCmd)).State

	return base, &image.Image{
		Image: ocispecs.Image{},
		Config: image.ImageConfig{
			ImageConfig: ocispecs.ImageConfig{},
		},
	}, nil
}
