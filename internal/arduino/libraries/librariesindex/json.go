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
	"bufio"
	"iter"

	"github.com/arduino/arduino-cli/internal/arduino/resources"
	"github.com/arduino/go-paths-helper"
	json "github.com/goccy/go-json"
	"go.bug.st/f"
	semver "go.bug.st/relaxed-semver"
)

type indexRelease struct {
	Name             string             `json:"name"`
	Version          *semver.Version    `json:"version"`
	Author           string             `json:"author"`
	Maintainer       string             `json:"maintainer"`
	Sentence         string             `json:"sentence"`
	Paragraph        string             `json:"paragraph"`
	Website          string             `json:"website"`
	Category         string             `json:"category"`
	Architectures    []string           `json:"architectures"`
	Types            []string           `json:"types"`
	URL              string             `json:"url"`
	ArchiveFileName  string             `json:"archiveFileName"`
	Size             int64              `json:"size"`
	Checksum         string             `json:"checksum"`
	Dependencies     []*indexDependency `json:"dependencies,omitempty"`
	License          string             `json:"license"`
	ProvidesIncludes []string           `json:"providesIncludes"`
}

type indexDependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// indexReleaseLite is a light projection of indexRelease with only the fields
// the dependency resolver and installer need. Decoding it skips the large
// descriptive fields, so resolving over the whole index stays cheap.
type indexReleaseLite struct {
	Name            string             `json:"name"`
	Version         *semver.Version    `json:"version"`
	Dependencies    []*indexDependency `json:"dependencies,omitempty"`
	URL             string             `json:"url"`
	ArchiveFileName string             `json:"archiveFileName"`
	Size            int64              `json:"size"`
	Checksum        string             `json:"checksum"`
}

// newDependencyResolver streams the index into a semver resolver, adding a
// light release for every library. The given overrides are used as-is instead
// of the index versions of the same libraries.
func (idx *Index) newDependencyResolver(overrides []*Release) *semver.Resolver[*Release, *Dependency] {
	resolver := semver.NewResolver[*Release]()

	overridden := map[string]bool{}
	for _, override := range overrides {
		resolver.AddRelease(override)
		overridden[override.GetName()] = true
	}

	if idx == nil || idx.indexFile == nil {
		return resolver
	}
	file, err := idx.indexFile.Open()
	if err != nil {
		return resolver
	}
	defer file.Close()

	dec := json.NewDecoder(bufio.NewReaderSize(file, 1024*1024))
	// Skip to inside the "libraries" array: '{', "libraries", '['.
	for range 3 {
		if _, err := dec.Token(); err != nil {
			return resolver
		}
	}
	for dec.More() {
		var r indexReleaseLite
		if dec.Decode(&r) != nil {
			break
		}
		if !overridden[r.Name] {
			resolver.AddRelease(r.toRelease())
		}
	}
	return resolver
}

// toRelease builds a *Release with the fields the resolver and installer need.
func (lite *indexReleaseLite) toRelease() *Release {
	return &Release{
		Version:      lite.Version,
		Dependencies: f.Map(lite.Dependencies, (*indexDependency).extractDependency),
		Resource: &resources.DownloadResource{
			URL:             lite.URL,
			ArchiveFileName: lite.ArchiveFileName,
			Size:            lite.Size,
			Checksum:        lite.Checksum,
			CachePath:       "libraries",
		},
		Library: &Library{Name: lite.Name},
	}
}

// LoadIndex creates an Index backed by the given library_index.json file.
// The file is not read here: it is streamed on demand by the Index methods, so
// that the whole (potentially very large) index is never held in memory.
func LoadIndex(indexFile *paths.Path) (*Index, error) {
	return &Index{indexFile: indexFile}, nil
}

// scanIndexFile streams the "libraries" array of the given index file, yielding
// one release at a time without ever loading the whole file. The file is closed
// when iteration ends; any read or decode error silently stops it.
func scanIndexFile(indexFile *paths.Path) iter.Seq[*indexRelease] {
	return func(yield func(*indexRelease) bool) {
		file, err := indexFile.Open()
		if err != nil {
			return
		}
		defer file.Close()

		dec := json.NewDecoder(bufio.NewReaderSize(file, 1024*1024))

		// The index has the form: { "libraries": [ <release>, <release>, ... ] }.
		if _, err := dec.Token(); err != nil { // consume the opening '{'
			return
		}
		for dec.More() {
			keyToken, err := dec.Token()
			if err != nil {
				return
			}
			if key, _ := keyToken.(string); key != "libraries" {
				// Skip any unexpected top-level field without loading it whole.
				var discard json.RawMessage
				if dec.Decode(&discard) != nil {
					return
				}
				continue
			}
			if _, err := dec.Token(); err != nil { // consume the opening '['
				return
			}
			for dec.More() {
				var release indexRelease
				if dec.Decode(&release) != nil {
					return
				}
				if !yield(&release) {
					return
				}
			}
			// The "libraries" array is the only field we care about.
			return
		}
	}
}

func (indexLib *indexRelease) extractReleaseIn(library *Library) {
	release := &Release{
		Version:       indexLib.Version,
		Author:        indexLib.Author,
		Maintainer:    indexLib.Maintainer,
		Sentence:      indexLib.Sentence,
		Paragraph:     indexLib.Paragraph,
		Website:       indexLib.Website,
		Category:      indexLib.Category,
		Architectures: indexLib.Architectures,
		Types:         indexLib.Types,
		Resource: &resources.DownloadResource{
			URL:             indexLib.URL,
			ArchiveFileName: indexLib.ArchiveFileName,
			Size:            indexLib.Size,
			Checksum:        indexLib.Checksum,
			CachePath:       "libraries",
		},
		Library:          library,
		Dependencies:     indexLib.extractDependencies(),
		License:          indexLib.License,
		ProvidesIncludes: indexLib.ProvidesIncludes,
	}
	library.Releases[indexLib.Version.NormalizedString()] = release
	if library.Latest == nil || library.Latest.Version.LessThan(release.Version) {
		library.Latest = release
	}
}

func (indexLib *indexRelease) extractDependencies() []*Dependency {
	res := []*Dependency{}
	if len(indexLib.Dependencies) == 0 {
		return res
	}
	for _, indexDep := range indexLib.Dependencies {
		res = append(res, indexDep.extractDependency())
	}
	return res
}

func (indexDep *indexDependency) extractDependency() *Dependency {
	var constraint semver.Constraint
	if c, err := semver.ParseConstraint(indexDep.Version); err == nil {
		constraint = c
	}
	// FIXME: else { report invalid constraint }
	return &Dependency{
		Name:              indexDep.Name,
		VersionConstraint: constraint,
	}
}
