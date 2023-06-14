package frontend

import (
	"fmt"
	"os"

	"github.com/moby/buildkit/client/llb"
	spb "github.com/moby/buildkit/sourcepolicy/pb"
	"sigs.k8s.io/yaml"
)

type Merger interface {
	Merge([]llb.State) llb.State
}

type CopyMerger struct{}

func (_ *CopyMerger) Merge(s []llb.State) llb.State {
	if len(s) == 0 {
		return llb.Scratch()
	}

	state := s[0]
	for i := 1; i < len(s); i++ {
		state = state.File(llb.Copy(s[i], "/", "/"))
	}

	return state
}

var cm Merger = &CopyMerger{}

type MergeMerger struct{}

func (_ *MergeMerger) Merge(s []llb.State) llb.State {
	return llb.Merge(s)
}

// Spec is the specification for a package build.
type Spec struct {
	// Name is the name of the package.
	Name string `json:"name"`
	// Description is a short description of the package.
	Description string `json:"description"`
	// Website is the URL to store in the metadata of the package.
	Website string `json:"website"`
	// Version is the package version. Must be a valid semver
	Version string `json:"version"`

	// Dependencies are the different dependencies that need to be specified in the package.
	Dependencies PackageDependencies `json:"dependencies"`

	// Conflicts is the list of packages that conflict with the generated package.
	// This will prevent the package from being installed if any of these packages are already installed or vice versa.
	Conflicts []string `json:"conflicts"`
	// Replaces is the list of packages that are replaced by the generated package.
	Replaces []string `json:"replaces"`
	// Provides is the list of things that the generated package provides.
	// This can be used to satisfy dependencies of other packages.
	// As an example, the moby-runc package provides "runc", other packages could depend on "runc" and be satisfied by moby-runc.
	// This is an advanced use case and consideration should be taken to ensure that the package actually provides the thing it claims to provide.
	Provides []string `json:"provides"`

	// Sources is the list of sources to use to build the artifact(s).
	// The map key is the name of the source and the value is the source configuration.
	// The source configuration is used to fetch the source and filter the files to include/exclude.
	// This can be mounted into the build using the "Mounts" field in the StepGroup.
	//
	// Sources can be embedded in the main spec as here or overriden in a build request.
	Sources map[string]Source `json:"sources"`

	// BuildSteps is the list of build steps to run to build the artifact(s).
	// Each entry may be run in parallel and will not share state with each other.
	BuildSteps []StepGroup `json:"buildSteps"`

	// SourcePolicy is used to approve/deny/rewrite sources used by a build.
	SourcePolicy *spb.Policy `json:"sourcePolicy"`
}

// Source defines a source to be used in the build.
// A source can be a local directory, a git repositoryt, http(s) URL, etc.
type Source struct {
	// Ref is a unique identifier for the source.
	// example: "docker-image://busybox:latest", "https://github.com/moby/moby.git#master", "local://some/local/path
	Ref string `json:"ref"`
	// Path is the path to the source after fetching it based on the identifier.
	Path string `json:"path"`
	// Filters is used to filter the files to include/exclude from beneath "Path".
	Filters `json:",inline"`
	// Satisfies is the list of build dependencies that this source satisfies.
	// This needs to match the name of the dependency in the PackageDependencies.Build list.
	Satisfies []string `json:"satisfies"`
}

// PackageDependencies is a list of dependencies for a package.
// This will be included in the package metadata so that the package manager can install the dependencies.
// It also includes build-time dedendencies, which we'll install before running any build steps.
type PackageDependencies struct {
	// Build is the list of packagese required to build the package.
	Build []string `json:"build"`
	// Runtime is the list of packages required to install/run the package.
	Runtime []string `json:"runtime"`
	// Recommends is the list of packages recommended to install with the generated package.
	// Note: Not all package managers support this (e.g. rpm)
	Recommends []string `json:"recommends"`
}

// StepGroup configures a group of steps that are run sequentially along with their outputs to build the artifact(s).
type StepGroup struct {
	// Steps is the list of commands to run to build the artifact(s).
	// Each step is run sequentially and will be cached accordingly.
	Steps []BuildStep `json:"steps"`
	// List of CacheDirs which will be used across all Steps
	CacheDirs map[string]CacheDirConfig `json:"cacheDirs"`
	// Outputs is the list of artifacts to be extracted after running the steps.
	Outputs map[string]ArtifactConfig `json:"outputs"`
	// Mounts is the list of sources to mount into the build.
	// The map key is the name of the source to mount and the value is the path to mount it to.
	Mounts map[string]string `json:"mounts"`
	// Workdir specifies the working directory that each new command will run in within this step group
	WorkDir string `json:"workDir"`
}

// BuildStep is used to execute a command to build the artifact(s).
type BuildStep struct {
	// Command is the command to run to build the artifact(s).
	// This will always be wrapped as /bin/sh -c "<command>", or whatever the equivalent is for the target distro.
	Command string `json:"command"`
	// CacheDirs is the list of CacheDirs which will be used for this build step.
	// Note that this list will be merged with the list of CacheDirs from the StepGroup.
	CacheDirs map[string]CacheDirConfig `json:"cacheDirs"`
}

// CacheDirConfig configures a persistent cache to be used across builds.
type CacheDirConfig struct {
	// Mode is the locking mode to set on the cache directory
	// values: shared, private, locked
	// default: shared
	Mode string `json:"mode"`
	// Key is the cache key to use to cache the directory
	// default: Value of `Path`
	Key string `json:"key"`
	// IncludeDistroKey is used to include the distro key as part of the cache key
	// What this key is depends on the frontend implementation
	// Example for Debian Buster may be "buster"
	IncludeDistroKey bool `json:"includeDistroKey"`
	// IncludeArchKey is used to include the architecture key as part of the cache key
	// What this key is depends on the frontend implementation
	// Frontends SHOULD use the buildkit platform arch
	IncludeArchKey bool `json:"includeArchKey"`
}

type ArtifactType string

var (
	// ArtifactTypeExecutable is used to install a binary
	ArtifactTypeExecutable ArtifactType = "exe"
	// ArtifactTypeManpage is used to install a manpage document
	ArtifactTypeManpage ArtifactType = "manpage"
	// ArtifactTypeSystemdUnit is used to install a systemd unit file
	ArtifactTypeSystemdUnit ArtifactType = "systemd-unit"
	// ArtifactTypePreInst is used to run a script after installing the package
	ArtifactTypePostInst ArtifactType = "postinst"
	// ArtifactTypeContrib is used to install a contrib file
	ArtifactTypeContrib ArtifactType = "contrib"
	// TODO: others TBD, eg config files, licenses, notices, libexec, etc
	ArtifactTypeText ArtifactType = "text"
)

// ArtifactConfig is used to configure how to extract an artifact and whatit is
type ArtifactConfig struct {
	// ArtifactType defines the type of artifact this is and will determine how to handle it in the package
	Type    ArtifactType `json:"type"`
	Filters `json:",inline"`
}

// Filters is used to filter the files to include/exclude from a directory.
type Filters struct {
	// Includes is a list of paths underneath `Path` to include, everything else is execluded
	// If empty, everything is included (minus the excludes)
	Includes []string `json:"includes"`
	// Excludes is a list of paths underneath `Path` to exclude, everything else is included
	Excludes []string `json:"excludes"`
}

func LoadSpec(dt []byte) (*Spec, error) {
	// cuectx := cuecontext.New()
	// v := cuectx.CompileBytes(dt)

	// var r cue.Runtime
	// var cfg gocodec.Config
	// codec := gocodec.New(&r, &cfg)

	var i int
	for i = 0; i < len(dt) && dt[i] != byte('\n'); i++ {
	}
	i++

	var spec Spec
	if err := yaml.Unmarshal(dt[i:], &spec); err != nil {
		return nil, err
	}
	fmt.Fprintln(os.Stderr, "spec", spec)
	// if err := codec.Encode(v, &spec); err != nil {
	// 	return nil, err
	// }

	return &spec, nil
}

// func LoadSpec2(dt []byte) (*Spec, error) {
// 	instances := load.Instances(nil, &load.Config{
// 		Package:     "bkpkg/frontend",
// 		AllCUEFiles: true,
// 	})
//
// }
