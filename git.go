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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	if len(os.Args) == 0 {
		panic("Cannot retrieve program name.")
	}
	if parentName() == `` {
		/* If we can't get the parent, then this won't work.
		 * Just move on and hope for the best.
		 */
		return
	}

	if git_ssh := os.Getenv(`GIT_SSH`); git_ssh != `` {
		err := os.Setenv(`TSB_SSH`, git_ssh)
		if err != nil {
			panic("Cannot set tsb ssh command: " + err.Error())
		}
	}
	err := os.Setenv(`GIT_SSH`, os.Args[0])
	if err != nil {
		panic("Cannot set ssh command: " + err.Error())
	}
}

func run(cmd string, args ...string) ([]byte, error) {
	if verbose {
		fmt.Fprintf(os.Stderr, "%s %s\n", cmd, strings.Join(args, ` `))
	}
	b, err := exec.Command(cmd, args...).Output()
	if verbose {
		fmt.Fprintf(os.Stderr, "|>\t%s\n", bytes.Replace(bytes.TrimSpace(b), []byte{'\n'}, []byte{'\n', '|', '>', '\t'}, -1))
	}
	return b, NewFailedCommand(err, cmd, args...)
}

func git(args ...string) ([]byte, error) {
	return run(`git`, args...)
}

type gitRepo string

func (r gitRepo) gitDir() string {
	return filepath.Join(string(r), `.git`)
}

func (r gitRepo) git(args ...string) ([]byte, error) {
	args = append([]string{`--git-dir=` + r.gitDir(), `--work-tree=` + string(r)}, args...)
	return git(args...)
}
