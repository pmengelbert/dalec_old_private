package mariner2

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/azure/dalec/frontend"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/exporter/containerimage/image"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
)

const imgRef = "mcr.microsoft.com/cbl-mariner/base/core:2.0"

const rpmSpec = `
Summary: A package built by DALEC
Name: %{name}
Version: %{_version}
Release: 1%{?dist}
License: Apache 2.0
Source: archive.tar.gz
URL: %{_website}
Vendor: Moby
Packager: Microsoft <support@microsoft.com>

%description
{{ .Description }}

%prep
%setup -q -n %{name}-%{version} -c

%install
cp -aLT ./archive/ %{buildroot}
find %{buildroot} | sed -E 's#^%{buildroot}##g' > ./files

%files -f ./files

%changelog
* Mon Jan 27 2020 Brian Goffs <brgoff@microsoft.com>
- Use dynamic linking and issue build commands from rpm spec
* Tue Aug 7 2018 Robledo Pontes <rafilho@microsoft.com>
- Adding to moby build tools.
* Mon Mar 12 2018 Xing Wu <xingwu@microsoft.com>
- First draft
`

type SourceState struct {
	frontend.Source
	s llb.State
}

func Convert(ctx context.Context, spec *frontend.Spec, merger frontend.Merger) (llb.State, *image.Image, error) {
	base := llb.Image(imgRef).
		Run(llb.Args([]string{
			"tdnf", "install", "-y", "build-essential", "rpmdevtools",
		})).State

	depsCmd := []string{"tdnf", "install", "-y"}
	for _, dep := range spec.Dependencies.Build {
		depsCmd = append(depsCmd, dep)
	}
	base = base.Run(llb.Args(depsCmd)).State

	sourceStates := make(map[string]SourceState)
	for name, src := range spec.Sources {
		if src.Ref != "" {
			scheme, ref, ok := strings.Cut(src.Ref, "://")
			if !ok {
				// treat as local file
				scheme = "local"
			}

			var s llb.State
			switch scheme {
			case "local":
				s = llb.Local("context", llb.FollowPaths([]string{ref}))
			case "http", "https":
				// include scheme
				s = llb.HTTP(src.Ref)
			case "git":
				repo, tag, _ := strings.Cut(ref, "#")
				s = llb.Git(repo, tag)
			case "docker-image":
				s = llb.Image(ref)
			default:
				return s, nil, fmt.Errorf("invalid scheme for source '%s': '%s'", name, scheme)
			}

			sourceStates[name] = SourceState{Source: src, s: s}
		}
	}

	diffs := []llb.State{base}
	for i := range spec.BuildSteps {
		stepBuild := base
		for name, dir := range spec.BuildSteps[i].Mounts {
			stepBuild = stepBuild.File(llb.Copy(sourceStates[name].s, sourceStates[name].Source.Path, dir, &llb.CopyInfo{
				CreateDestPath: true,
			}))
		}

		wd := spec.BuildSteps[i].WorkDir
		stepBuild = stepBuild.File(llb.Mkdir(wd, 0o755, llb.WithParents(true)))
		stepBuild = stepBuild.Dir(wd)

		for j := range spec.BuildSteps[i].Steps {
			stepBuild = stepBuild.Run(llb.Args([]string{"/usr/bin/env", "sh", "-c", spec.BuildSteps[i].Steps[j].Command})).State
		}

		diffs = append(diffs, llb.Diff(base, stepBuild))
	}

	build := merger.Merge(diffs)

	scratch := llb.Scratch()
	outs := []llb.State{scratch}
	for i := range spec.BuildSteps {
		stepOut := scratch
		for path, output := range spec.BuildSteps[i].Outputs {
			switch output.Type {
			case frontend.ArtifactTypeExecutable:
				stepOut = outputExe(path, stepOut, output, build)
			case frontend.ArtifactTypeText:
				stepOut = outputText(path, stepOut, output, build)
			}
		}
		outs = append(outs, llb.Diff(scratch, stepOut))
	}

	out := merger.Merge(outs)

	pkgState := buildPackage(build, out, spec)

	return pkgState, &image.Image{
		Image: ocispecs.Image{},
		Config: image.ImageConfig{
			ImageConfig: ocispecs.ImageConfig{},
		},
	}, nil
}

func outputExe(path string, out llb.State, output frontend.ArtifactConfig, build llb.State) llb.State {
	for _, incl := range output.Includes {
		out = out.File(llb.Copy(build, incl, path, &llb.CopyInfo{CreateDestPath: true, AllowWildcard: true}))
	}
	return out
}

func outputText(path string, out llb.State, output frontend.ArtifactConfig, build llb.State) llb.State {
	for _, incl := range output.Includes {
		out = out.File(llb.Copy(build, incl, path, &llb.CopyInfo{CreateDestPath: true, AllowWildcard: true}))
	}
	return out
}

func buildPackage(base, in llb.State, spec *frontend.Spec) llb.State {
	build := base.Dir("/build")
	build = build.File(llb.Mkdir("/build/SOURCES", 0o755, llb.WithParents(true)))
	build = build.File(llb.Mkdir("/build/archive", 0o755, llb.WithParents(true)))
	build = build.File(llb.Copy(in, "/", "/build/archive"))

	build = build.Run(llb.Args([]string{
		"tar", "-czf", "./SOURCES/archive.tar.gz", "archive/",
	})).State

	// Because the description can be multiline, it has to be injected rather
	// than passed in on the command line.
	t, _ := template.New("spec").Parse(rpmSpec)
	b := new(bytes.Buffer)
	t.Execute(b, spec)
	build = build.File(llb.Mkfile("/build/pkg.spec", 0o644, b.Bytes()))

	// The rest of the variables can use the native rpmbuild templating system.
	build = build.Run(llb.Args([]string{
		"rpmbuild", "-bb",
		"--define", rpmOpt("_topdir", "/build"),
		"--define", rpmOpt("name", spec.Name),
		"--define", rpmOpt("_version", spec.Version),
		"--define", rpmOpt("_website", spec.Website),
		"pkg.spec",
	})).State

	build = build.Dir("/out")
	build = build.Run(llb.Args([]string{
		"find", "/build/RPMS", "-name", "*.rpm", "-exec", "mv", "{}", "/out", ";",
	})).State

	return llb.Scratch().File(llb.Copy(build, "/out", "/", &llb.CopyInfo{CopyDirContentsOnly: true}))
}

func rpmOpt(k, v string) string {
	return fmt.Sprintf("%%%s %s", k, v)
}
