/*
 * This file is part of arduino-cli.
 *
 * Copyright 2018 ARDUINO AG (http://www.arduino.cc/)
 *
 * arduino-cli is free software; you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation; either version 2 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin St, Fifth Floor, Boston, MA  02110-1301  USA
 *
 * As a special exception, you may use this file as part of a free software
 * library without restriction.  Specifically, if other files instantiate
 * templates or use macros or inline functions from this file, or you compile
 * this file and link it with other files to produce an executable, this
 * file does not by itself cause the resulting executable to be covered by
 * the GNU General Public License.  This exception does not however
 * invalidate any other reasons why the executable file might be covered by
 * the GNU General Public License.
 */

package output

import (
	"fmt"
	"sort"

	"github.com/bcmi-labs/arduino-cli/arduino/libraries/librariesindex"
	semver "go.bug.st/relaxed-semver"

	"github.com/bcmi-labs/arduino-cli/arduino/libraries"
	"github.com/gosuri/uitable"
)

// InstalledLibraries is a list of installed libraries
type InstalledLibraries struct {
	Libraries []*InstalledLibary `json:"libraries"`
}

// InstalledLibary is an installed library
type InstalledLibary struct {
	Library   *libraries.Library      `json:"library"`
	Available *librariesindex.Release `omitempy,json:"available"`
}

func (il InstalledLibraries) Len() int { return len(il.Libraries) }
func (il InstalledLibraries) Swap(i, j int) {
	il.Libraries[i], il.Libraries[j] = il.Libraries[j], il.Libraries[i]
}
func (il InstalledLibraries) Less(i, j int) bool {
	return il.Libraries[i].Library.String() < il.Libraries[j].Library.String()
}

func (il InstalledLibraries) String() string {
	table := uitable.New()
	table.MaxColWidth = 100
	table.Wrap = true

	hasUpdates := false
	for _, libMeta := range il.Libraries {
		if libMeta.Available != nil {
			hasUpdates = true
		}
	}

	if hasUpdates {
		table.AddRow("Name", "Installed", "Available", "Location")
	} else {
		table.AddRow("Name", "Installed", "Location")
	}
	sort.Sort(il)
	lastName := ""
	for _, libMeta := range il.Libraries {
		lib := libMeta.Library
		name := lib.Name
		if name == lastName {
			name = ` "`
		} else {
			lastName = name
		}

		location := lib.Location.String()
		if lib.ContainerPlatform != nil {
			location = lib.ContainerPlatform.String()
		}
		if hasUpdates {
			var available *semver.Version
			if libMeta.Available != nil {
				available = libMeta.Available.Version
			}
			table.AddRow(name, lib.Version, available, location)
		} else {
			table.AddRow(name, lib.Version, location)
		}
	}
	return fmt.Sprintln(table)
}