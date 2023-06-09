package mariner2

import (
	"context"
	"path/filepath"
	"strings"

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
	for _, dep := range spec.Dependencies.Build {
		depsCmd = append(depsCmd, dep)
	}
	base = base.Run(llb.Args(depsCmd)).State

	sourceStates := make(map[string]llb.State)
	for name, src := range spec.Sources {
		if src.Ref != "" {
			repo, tag, _ := strings.Cut(src.Ref, "#")
			gitState := llb.Git(repo, tag)
			sourceStates[name] = gitState
		}
	}

	out := llb.Scratch()
	for i := range spec.BuildSteps {
		for name, dir := range spec.BuildSteps[i].Mounts {
			base = base.File(llb.Copy(sourceStates[name], "/", dir, &llb.CopyInfo{
				CreateDestPath: true,
			}))
		}

		wd := spec.BuildSteps[i].WorkDir
		base = base.File(llb.Mkdir(wd, 0o755, llb.WithParents(true)))
		base = base.Dir(spec.BuildSteps[i].WorkDir)

		for j := range spec.BuildSteps[i].Steps {
			base = base.Run(llb.Args([]string{"/usr/bin/env", "sh", "-c", spec.BuildSteps[i].Steps[j].Command})).State
		}

		for path, output := range spec.BuildSteps[i].Outputs {
			if output.Type != frontend.ArtifactTypeExecutable {
				continue
			}

			dstDir := filepath.Dir(path)
			out = out.File(llb.Mkdir(dstDir, 0o755, llb.WithParents(true)))
			for _, incl := range output.Includes {
				out = out.File(llb.Copy(base, incl, path))
			}
		}
	}

	return out, &image.Image{
		Image: ocispecs.Image{},
		Config: image.ImageConfig{
			ImageConfig: ocispecs.ImageConfig{},
		},
	}, nil
}
