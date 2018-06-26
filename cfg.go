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
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
)

type Config struct {
	Repos   Repos   `file:"yaml,repos.yml"`
	Patches Patches `file:"yaml,patches.yml"`
	Compose Compose `file:"yaml,docker-compose.yml"`
}

type Repos map[string]*Repo

type Repo struct {
	Source string   `yaml:"src"`
	Branch string   `yaml:"branch"`
	Head   string   `yaml:"head"`
	Extra  []string `yaml:"extra"`
}

type Patches map[string][]string

func (rs Repos) forAllRepos(f func(r *Repo, dir, name string) error, dir string) error {
	var wg sync.WaitGroup
	var errLock sync.Mutex
	var errs []error

	wg.Add(len(rs))
	for name, repo := range rs {
		go func(name string, repo *Repo) {
			defer wg.Done()
			err := f(repo, dir, name)
			if err != nil {
				errLock.Lock()
				errs = append(errs, err)
				errLock.Unlock()
			}
		}(name, repo)
	}
	wg.Wait()

	if len(errs) > 0 {
		var errmsg string
		for _, err := range errs {
			errmsg += err.Error() + "\n"
		}
		return errors.New(errmsg)
	}
	return nil
}

func (rs Repos) Fetch(dir string) error {
	return rs.forAllRepos((*Repo).Fetch, dir)
}

func (rs Repos) Update(dir string) error {
	return rs.forAllRepos((*Repo).Update, dir)
}

func (rs Repos) Prepare(dir string) error {
	return rs.forAllRepos((*Repo).Prepare, dir)
}

func (r *Repo) Fetch(dir string, name string) error {
	srcDir := filepath.Join(dir, `src`)
	repoDir := filepath.Join(srcDir, name)
	gitDir := filepath.Join(repoDir, `.git`)

	/* Clone the repo if it isn't already there. */
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		_ = os.RemoveAll(repoDir)
		_, err := git(`clone`, `--no-checkout`, r.Source, repoDir)
		if err != nil {
			return err
		}
	}

	gr := gitRepo(repoDir)

	/* Configure the remotes to match the repo spec. */
	existingRemotes, err := gr.git(`remote`)
	if err != nil {
		return err
	}

	remoteBag := make(map[string]struct{})
	for _, existingRemote := range bytes.Split(existingRemotes, []byte{'\n'}) {
		er := bytes.Replace(existingRemote, []byte{'\r'}, []byte{}, -1)
		if len(er) != 0 {
			remoteBag[string(er)] = struct{}{}
		}
	}

	err = SetRemote(gr, `origin`, r.Source)
	if err != nil {
		return err
	}
	fmt.Println("Marking origin used.")
	delete(remoteBag, `origin`)
	for i, extra := range r.Extra {
		rmtName := fmt.Sprintf(`extra%02d`, i)
		err := SetRemote(gr, rmtName, extra)
		if err != nil {
			return err
		}
		fmt.Println("Marking " + rmtName + " used.")
		delete(remoteBag, rmtName)
	}

	for unusedRemote := range remoteBag {
		_, err := gr.git(`remote`, `remove`, unusedRemote)
		if err != nil {
			return err
		}
	}

	_, err = gr.git(`fetch`, `--all`)
	if err != nil {
		return err
	}

	return nil
}

func SetRemote(repo gitRepo, remote, target string) error {
	wrap := func(err error) error {
		return errors.New(`Unable to set remote ` + remote + ` to ` + target + ` for ` + string(repo) + `: ` + err.Error())
	}

	oldTargetBuf, err := repo.git(`ls-remote`, `--get-url`, remote)
	if err != nil {
		return wrap(err)
	}

	oldTarget := string(bytes.TrimSpace(oldTargetBuf))
	if target == oldTarget {
		/* Target is already set properly, do not disturb it. */
		return nil
	}

	/* ls-remote --get-url returns the input if the remote doesn't exist */
	if oldTarget != remote {
		_, err = repo.git(`remote`, `remove`, remote)
		if err != nil {
			return wrap(err)
		}
	}

	_, err = repo.git(`remote`, `add`, remote, target)
	if err != nil {
		return wrap(err)
	}

	return nil
}

func (r *Repo) Update(dir, name string) error {
	repo := gitRepo(filepath.Join(dir, `src`, name))
	newhead, err := repo.git(`log`, `-n1`, `--format=%H`, path.Join(`origin`, r.Branch))
	if err != nil {
		return errors.New(`Unable to get head of branch origin/` + r.Branch + `: ` + err.Error())
	}
	newhead = bytes.TrimSpace(newhead)
	r.Head = string(newhead)
	return nil
}

func (r *Repo) Prepare(dir, name string) error {
	repo := gitRepo(filepath.Join(dir, `src`, name))
	_, _ = repo.git(`clean`, `-dfx`) /* Don't complain about a failed clean, checkout will complain if necessary. */
	_, err := repo.git(`checkout`, `-f`, r.Head)
	if err != nil {
		return errors.New(`Unable to check out head ` + err.Error())
	}
	return nil
}

func (r *Repo) Cherry(dir, name, changeset string) error {
	repo := gitRepo(filepath.Join(dir, `src`, name))
	_, err := repo.git(`cherry-pick`, changeset)
	return err
}
