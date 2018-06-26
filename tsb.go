/*
Copyright 2017 Comcast Corporation

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
			break
		}
	}
}
