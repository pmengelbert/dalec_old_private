package rpmbundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/azure/dalec/frontend"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/image"
	gwclient "github.com/moby/buildkit/frontend/gateway/client"
	"github.com/moby/buildkit/solver/pb"
)

type getDigestFunc func(intput llb.State) (string, string, error)

func getDigestFromClientFn(ctx context.Context, client gwclient.Client) getDigestFunc {
	return func(input llb.State) (name string, dgst string, _ error) {
		st := marinerBase.Run(
			llb.AddMount("/tmp/st", input, llb.Readonly),
			llb.Dir("/tmp/st"),
			shArgs("set -e -o pipefail; sha256sum * >> /digest"),
		).State

		def, err := llb.Diff(marinerBase, st).Marshal(ctx)
		if err != nil {
			return "", "", err
		}

		res, err := client.Solve(ctx, gwclient.SolveRequest{
			Definition: def.ToPB(),
		})
		if err != nil {
			return "", "", err
		}
		dt, err := res.Ref.ReadFile(ctx, gwclient.ReadRequest{
			Filename: "/digest",
		})
		if err != nil {
			return "", "", err
		}

		// Format is `<hash> <filename>`
		split := bytes.Fields(bytes.TrimSpace(dt))
		return string(split[1]), string(split[0]), nil
	}
}

func handleMariner2Buildroot(ctx context.Context, client gwclient.Client, spec *frontend.Spec) (gwclient.Reference, *image.Image, error) {
	caps := client.BuildOpts().LLBCaps
	noMerge := !caps.Contains(pb.CapMergeOp)

	st, err := specToMariner2BuildrootLLB(spec, noMerge, getDigestFromClientFn(ctx, client))
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

func specToMariner2BuildrootLLB(spec *frontend.Spec, noMerge bool, getDigest getDigestFunc) (llb.State, error) {
	specs, err := specToRpmSpecLLB(spec, llb.Scratch(), "/")
	if err != nil {
		return llb.Scratch(), err
	}

	sources, err := specToSourcesLLB(spec)
	if err != nil {
		return llb.Scratch(), err
	}

	inputs := append(sources, specs)

	// The mariner toolkit wants a signatures file in the spec dir (next to the spec file) that contains the sha256sum of all sources.
	sigs := make(map[string]string, len(sources))
	for _, src := range sources {
		fName, dgst, err := getDigest(src)
		if err != nil {
			return llb.Scratch(), fmt.Errorf("could not get digest for source: %w", err)
		}
		sigs[fName] = dgst
	}

	type sigData struct {
		Signatures map[string]string `json:"Signatures"`
	}

	var sd sigData
	sd.Signatures = sigs
	dt, err := json.Marshal(sd)
	if err != nil {
		return llb.Scratch(), fmt.Errorf("could not marshal signatures: %w", err)
	}
	inputs = append(inputs, llb.Scratch().File(
		llb.Mkfile(spec.Name+".signatures.json", 0600, dt),
	))

	return mergeOrCopy(llb.Scratch(), inputs, filepath.Join("/SPECS", spec.Name), noMerge), nil
}