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
	"os/exec"
	"path"
	"syscall"
)

func sshForGit() {
	if parentName() == `git` {
		ssh, err := exec.LookPath(`ssh`)
		if err != nil {
			panic(err)
		}
		ssh_git := os.Getenv(`TSB_SSH`)
		if ssh_git == `` {
			ssh_git = `ssh`
		}
		/* Since tsb needs to run from non-interactive hosts, we can't rely on human checking of the keys. :( */
		args := append([]string{
			ssh_git,
			`-o`, `UserKnownHostsFile=/dev/null`,
			`-o`, `StrictHostKeyChecking=no`},
			os.Args[1:]...,
		)
		// fmt.Fprintf(os.Stderr, "Invoking %v %v\n", ssh, args)
		panic(syscall.Exec(ssh, args, syscall.Environ()))
	}
}

func parentName() string {
	ppid := os.Getppid()
	name, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", ppid))
	if err != nil {
		return ``
	}

	return path.Base(name)
}
