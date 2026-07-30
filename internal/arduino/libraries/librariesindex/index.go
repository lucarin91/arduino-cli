// This file is part of arduino-cli.
//
// Copyright 2020 ARDUINO SA (http://www.arduino.cc/)
//
// This software is released under the GNU General Public License version 3,
// which covers the main part of arduino-cli.
// The terms of this license can be found at:
// https://www.gnu.org/licenses/gpl-3.0.en.html
//
// You can be released from the requirements of the above licenses by purchasing
// a commercial license. Buying such a license is mandatory if you want to
// modify or otherwise use the software for commercial activities involving the
// Arduino software without disclosing the source code of your own applications.
// To purchase a commercial license, send an email to license@arduino.cc.

package librariesindex

import (
	"iter"
	"sort"
	"strings"

	"github.com/arduino/arduino-cli/commands/cmderrors"
	"github.com/arduino/arduino-cli/internal/arduino/libraries"
	"github.com/arduino/arduino-cli/internal/arduino/resources"
	rpc "github.com/arduino/arduino-cli/rpc/cc/arduino/cli/commands/v1"
	"github.com/arduino/go-paths-helper"
	semver "go.bug.st/relaxed-semver"
)

// Index represents the list of libraries available for download. The library
// data is not kept in memory: it is streamed from the backing
// library_index.json file on demand by the methods below.
type Index struct {
	indexFile *paths.Path
}

// EmptyIndex is an empty library index
var EmptyIndex = &Index{}

// Library is a library available for download
type Library struct {
	Name     string
	Releases map[semver.NormalizedString]*Release
	Latest   *Release `json:"-"`
	Index    *Index   `json:"-"`
}

// Release is a release of a library available for download
type Release struct {
	Author           string
	Version          *semver.Version
	Dependencies     []*Dependency
	Maintainer       string
	Sentence         string
	Paragraph        string
	Website          string
	Category         string
	Architectures    []string
	Types            []string
	Resource         *resources.DownloadResource
	License          string
	ProvidesIncludes []string

	Library *Library `json:"-"`
}

// ToRPCLibraryRelease transform this Release into a rpc.LibraryRelease
func (r *Release) ToRPCLibraryRelease() *rpc.LibraryRelease {
	return &rpc.LibraryRelease{
		Author:        r.Author,
		Version:       r.Version.String(),
		Maintainer:    r.Maintainer,
		Sentence:      r.Sentence,
		Paragraph:     r.Paragraph,
		Website:       r.Website,
		Category:      r.Category,
		Architectures: r.Architectures,
		Types:         r.Types,
	}
}

// GetName returns the name of this library.
func (r *Release) GetName() string {
	return r.Library.Name
}

// GetVersion returns the version of this library.
func (r *Release) GetVersion() *semver.Version {
	return r.Version
}

// GetDependencies returns the dependencies of this library.
func (r *Release) GetDependencies() []*Dependency {
	return r.Dependencies
}

// ReleaseCompare compares two library releases by name, or by version if the names are equal.
func ReleaseCompare(r1, r2 *Release) int {
	if cmp := strings.Compare(r1.GetName(), r2.GetName()); cmp != 0 {
		return cmp
	}
	return r1.GetVersion().CompareTo(r2.GetVersion())
}

// Dependency is a library dependency
type Dependency struct {
	Name              string
	VersionConstraint semver.Constraint
}

// GetName returns the name of the dependency
func (r *Dependency) GetName() string {
	return r.Name
}

// GetConstraint returns the version Constraint of the dependecy
func (r *Dependency) GetConstraint() semver.Constraint {
	return r.VersionConstraint
}

func (r *Release) String() string {
	return r.Library.Name + "@" + r.Version.String()
}

// releases streams the releases in the backing index file. It yields nothing on
// an empty index.
func (idx *Index) releases() iter.Seq[*indexRelease] {
	if idx == nil || idx.indexFile == nil {
		return func(yield func(*indexRelease) bool) {}
	}
	return scanIndexFile(idx.indexFile)
}

// findLibraries collects the requested libraries (by name), with all their
// releases, in a single scan of the index file.
func (idx *Index) findLibraries(names map[string]bool) map[string]*Library {
	libs := map[string]*Library{}
	if len(names) == 0 {
		return libs
	}
	for r := range idx.releases() {
		if !names[r.Name] {
			continue
		}
		lib := libs[r.Name]
		if lib == nil {
			lib = &Library{Name: r.Name, Releases: map[semver.NormalizedString]*Release{}}
			libs[r.Name] = lib
		}
		r.extractReleaseIn(lib)
	}
	return libs
}

// findLibrary collects a single library (with all its releases) from the index.
func (idx *Index) findLibrary(name string) *Library {
	return idx.findLibraries(map[string]bool{name: true})[name]
}

// FindIndexedLibraries returns the indexed libraries matching the given names,
// loaded in a single scan of the index file.
func (idx *Index) FindIndexedLibraries(names []string) map[string]*Library {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return idx.findLibraries(set)
}

// Libraries streams the index, yielding each library with all its releases
// populated. Only one library at a time is held in memory. This relies on the
// index grouping all the releases of a library contiguously (as produced by the
// Arduino library index generator).
func (idx *Index) Libraries() iter.Seq[*Library] {
	return func(yield func(*Library) bool) {
		var current *Library
		for r := range idx.releases() {
			if current != nil && current.Name != r.Name {
				if !yield(current) {
					return
				}
				current = nil
			}
			if current == nil {
				current = &Library{Name: r.Name, Releases: map[semver.NormalizedString]*Release{}}
			}
			r.extractReleaseIn(current)
		}
		if current != nil {
			yield(current)
		}
	}
}

// HasLibrary returns true if a library with the given name exists in the index.
func (idx *Index) HasLibrary(name string) bool {
	return idx.findLibrary(name) != nil
}

// FindRelease search a library Release in the index. Returns nil if the
// release is not found. If the version is not specified returns the latest
// version available.
func (idx *Index) FindRelease(name string, version *semver.Version) (*Release, error) {
	if library := idx.findLibrary(name); library != nil {
		if version == nil {
			return library.Latest, nil
		}
		if release, exists := library.Releases[version.NormalizedString()]; exists {
			return release, nil
		}
	}
	if version == nil {
		return nil, &cmderrors.LibraryNotFoundError{Library: name + "@latest"}
	}
	return nil, &cmderrors.LibraryNotFoundError{Library: name + "@" + version.String()}
}

// FindIndexedLibrary search an indexed library that matches the provided
// installed library or nil if not found
func (idx *Index) FindIndexedLibrary(lib *libraries.Library) *Library {
	return idx.findLibrary(lib.Name)
}

// FindLibraryUpdate check if an installed library may be updated using
// one of the indexed libraries. This function returns the Release to install
// to update the library if found, otherwise nil is returned.
func (idx *Index) FindLibraryUpdate(lib *libraries.Library) *Release {
	return libraryUpdate(idx.FindIndexedLibrary(lib), lib)
}

// FindLibraryUpdates checks, for each of the given installed libraries, whether
// an update is available in the index. The lookup is performed in a single scan
// of the index file. The returned map is keyed by the installed library and
// only contains entries for libraries that have an available update.
func (idx *Index) FindLibraryUpdates(libs []*libraries.Library) map[*libraries.Library]*Release {
	names := make([]string, len(libs))
	for i, lib := range libs {
		names[i] = lib.Name
	}
	indexed := idx.FindIndexedLibraries(names)
	updates := map[*libraries.Library]*Release{}
	for _, lib := range libs {
		if update := libraryUpdate(indexed[lib.Name], lib); update != nil {
			updates[lib] = update
		}
	}
	return updates
}

// libraryUpdate returns the release to update `lib` to, given its indexed
// counterpart `indexLib` (possibly nil), or nil if no update is available.
func libraryUpdate(indexLib *Library, lib *libraries.Library) *Release {
	if indexLib == nil {
		return nil
	}
	// If a library.properties has an invalid version property, usually empty or malformed,
	// the latest available version is returned
	if lib.Version == nil || indexLib.Latest.Version.GreaterThan(lib.Version) {
		return indexLib.Latest
	}
	return nil
}

// ResolveDependencies resolve the dependencies of a library release and returns a
// possible solution (the set of library releases to install together with the library).
// An optional "override" releases may be passed if we want to exclude the same
// libraries from the index (for example if we want to keep an installed library).
func (idx *Index) ResolveDependencies(lib *Release, overrides []*Release) []*Release {
	resolver := semver.NewResolver[*Release]()

	// done tracks library names already handled (added to the resolver, or
	// deliberately excluded); frontier holds the names to load in the next scan.
	done := map[string]bool{}
	frontier := map[string]bool{}
	enqueueDeps := func(deps []*Dependency) {
		for _, dep := range deps {
			if name := dep.GetName(); !done[name] {
				frontier[name] = true
			}
		}
	}

	// Overridden libraries are provided as-is and must not be taken from the
	// index; mark them done so they are never scanned, but still follow their
	// dependencies.
	for _, override := range overrides {
		resolver.AddRelease(override)
		done[override.GetName()] = true
	}
	for _, override := range overrides {
		enqueueDeps(override.Dependencies)
	}

	// Seed the resolver with the target library. Its releases are already
	// loaded (the caller obtained `lib` via FindRelease).
	if lib.Library != nil {
		done[lib.Library.Name] = true
		for _, release := range lib.Library.Releases {
			resolver.AddRelease(release)
			enqueueDeps(release.Dependencies)
		}
	}

	// Collect the transitive dependency closure, scanning the index once per
	// dependency-tree level. Only the libraries actually involved in the
	// resolution are loaded into memory, instead of the whole index.
	for len(frontier) > 0 {
		wanted := frontier
		frontier = map[string]bool{}
		for name := range wanted {
			done[name] = true
		}
		for _, indexLib := range idx.findLibraries(wanted) {
			for _, release := range indexLib.Releases {
				resolver.AddRelease(release)
				enqueueDeps(release.Dependencies)
			}
		}
	}

	// Perform lib resolution
	return resolver.Resolve(lib)
}

// Versions returns an array of all versions available of the library
func (library *Library) Versions() []*semver.Version {
	res := semver.List{}
	for _, release := range library.Releases {
		res = append(res, release.Version)
	}
	sort.Sort(res)
	return res
}
