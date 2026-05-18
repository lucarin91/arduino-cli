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
package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/arduino/arduino-cli/internal/cli/feedback"
	rpc "github.com/arduino/arduino-cli/rpc/cc/arduino/cli/commands/v1"
	"github.com/spf13/cobra"
	"go.bug.st/f"
)

// NewCommand creates a new `env` command
func NewCommand(srv rpc.ArduinoCoreServiceServer, settings *rpc.Configuration) *cobra.Command {
	return &cobra.Command{
		Use:   "env [SHELL]",
		Short: "Set environment variables for the Arduino CLI",
		Run: func(cmd *cobra.Command, args []string) {
			shell := "bash"
			if len(args) == 1 {
				shell = args[0]
			} else if s := filepath.Base(os.Getenv("SHELL")); s != "" && s != "." {
				shell = s
			}
			runEnv(shell, settings)
		},
	}
}

func runEnv(shell string, settings *rpc.Configuration) {
	dataDir := settings.GetDirectories().GetData()
	pattern := filepath.Join(dataDir, "packages", "*", "tools", "*", "*")
	matches := f.Must(filepath.Glob(pattern))
	if len(matches) == 0 {
		return
	}

	switch shell {
	case "fish":
		fmt.Printf("set -x PATH %s $PATH\n", strings.Join(matches, " "))
	case "bash", "zsh":
		fmt.Printf("export PATH=\"%s:$PATH\"\n", strings.Join(matches, ":"))
	default:
		feedback.Fatal("unsupported shell", feedback.ErrBadArgument)
	}
}
