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

package loadfiles

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

type File interface {
	Open() (io.ReadCloser, error)
	Create() (io.WriteCloser, error)
	In(filename string) File
	String() string
}

type OsFile string

func (f OsFile) Open() (io.ReadCloser, error) {
	return os.Open(string(f))
}

func (f OsFile) Create() (io.WriteCloser, error) {
	return os.Create(string(f))
}

func (f OsFile) In(filename string) File {
	return OsFile(filepath.Join(string(f), filename))
}

func (f OsFile) String() string {
	return string(f)
}

type GitFile struct {
	Repo string
	Rev  string
	Path string
}

func (f GitFile) Open() (io.ReadCloser, error) {
	b, err := exec.Command(`git`, `--git-dir=`+f.Repo, `show`, f.Rev+`:`+f.Path).CombinedOutput()
	if err != nil {
		return nil, errors.New(`Unable to get git file ` + f.Rev + `:` + f.Path + ` from ` + f.Repo + "\n" + string(b))
	}
	return ioutil.NopCloser(bytes.NewReader(b)), nil
}

func (f GitFile) Create() (io.WriteCloser, error) {
	return nil, errors.New("Cannot write to git file.")
}

func (f GitFile) In(filename string) File {
	return GitFile{
		Repo: f.Repo,
		Rev:  f.Rev,
		Path: path.Join(f.Path, filename),
	}
}

func (f GitFile) String() string {
	return f.Repo + `→` + f.Path + `@` + f.Rev
}
