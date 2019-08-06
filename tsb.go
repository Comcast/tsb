/*
Copyright 2018 Comcast Cable Communications Management, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"
)

var verbose bool

func main() {
	sshForGit()

	var ex Executor
	ex.cmds = os.Args[1:]

	for ex.HasArg() {
		/* fmt.Fprintf(os.Stderr, "Executing command: %v\n", ex.cmds[0]) */
		err := ex.Execute()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			/* For now just adding an error exit code. This can be done with more finese in the future
			   with the proper code passed around but this works for now */
			os.Exit(1)
			break
		}
	}
}
