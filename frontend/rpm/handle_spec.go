package rpm

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime/debug"

	"github.com/Azure/dalec"
	"github.com/Azure/dalec/frontend"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/image"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
)

func SpecHandler(target string) frontend.BuildFunc {
	return func(ctx context.Context, client gwclient.Client, spec *dalec.Spec) (gwclient.Reference, *image.Image, error) {
		st, err := Dalec2SpecLLB(spec, llb.Scratch(), target, "")
		if err != nil {
			return nil, nil, err
		}

		def, err := st.Marshal(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("error marshalling llb: %w", err)
		}

		res, err := client.Solve(ctx, gwclient.SolveRequest{
			Definition: def.ToPB(),
		})
		if err != nil {
			return nil, nil, err
		}
		ref, err := res.SingleRef()
		// Do not return a nil image, it may cause a panic
		return ref, &image.Image{}, err
	}
}

func Dalec2SpecLLB(spec *dalec.Spec, in llb.State, target, dir string) (llb.State, error) {
	buf := bytes.NewBuffer(nil)
	info, _ := debug.ReadBuildInfo()
	buf.WriteString("# Automatically generated by " + info.Main.Path + "\n")
	buf.WriteString("\n")

	if err := WriteSpec(spec, target, buf); err != nil {
		return llb.Scratch(), err
	}

	if dir == "" {
		dir = "SPECS/" + spec.Name
	}

	return in.
			File(llb.Mkdir(dir, 0755, llb.WithParents(true))).
			File(llb.Mkfile(filepath.Join(dir, spec.Name)+".spec", 0640, buf.Bytes())),
		nil
}
