package mariner2

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
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

	build := base
	out := llb.Scratch()
	for i := range spec.BuildSteps {
		for name, dir := range spec.BuildSteps[i].Mounts {
			build = build.File(llb.Copy(sourceStates[name], "/", dir, &llb.CopyInfo{
				CreateDestPath: true,
			}))
		}

		wd := spec.BuildSteps[i].WorkDir
		build = build.File(llb.Mkdir(wd, 0o755, llb.WithParents(true)))
		build = build.Dir(wd)

		for j := range spec.BuildSteps[i].Steps {
			build = build.Run(llb.Args([]string{"/usr/bin/env", "sh", "-c", spec.BuildSteps[i].Steps[j].Command})).State
		}

		for path, output := range spec.BuildSteps[i].Outputs {
			if output.Type != frontend.ArtifactTypeExecutable {
				continue
			}

			dstDir := filepath.Dir(path)
			out = out.File(llb.Mkdir(dstDir, 0o755, llb.WithParents(true)))
			for _, incl := range output.Includes {
				out = out.File(llb.Copy(build, incl, path))
			}
		}
	}

	pkgState := buildPackage(base, out, spec)

	return pkgState, &image.Image{
		Image: ocispecs.Image{},
		Config: image.ImageConfig{
			ImageConfig: ocispecs.ImageConfig{},
		},
	}, nil
}

func buildPackage(base, in llb.State, spec *frontend.Spec) llb.State {
	build := base.Dir("/build")
	build = build.File(llb.Mkdir("/build/SOURCES", 0o755, llb.WithParents(true)))
	build = build.File(llb.Mkdir("/build/archive", 0o755, llb.WithParents(true)))
	build = build.File(llb.Copy(in, "/", "/build/archive"))

	build = build.Run(llb.Args([]string{
		"tar", "-czf", "./SOURCES/archive.tar.gz", "archive/",
	})).State

	t, _ := template.New("spec").Parse(rpmSpec)
	b := new(bytes.Buffer)
	t.Execute(b, spec)
	build = build.File(llb.Mkfile("/build/pkg.spec", 0o644, b.Bytes()))

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
