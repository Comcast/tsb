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
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Repos   Repos   `file:"yaml,repos.yml"`
	Patches Patches `file:"yaml,patches.yml"`
	Compose Compose `file:"yaml,docker-compose.yml"`
}

type Repos map[string]*Repo

type Repo struct {
	Source string  `yaml:"src"`
	Branch string  `yaml:"branch,omitempty"`
	Tag    string  `yaml:"tag,omitempty"`
	Head   string  `yaml:"head"`
	Extras []Extra `yaml:"extra"`
}

type Subscription struct {
	Branch     string      `yaml:"branch"`
	Changesets []Changeset `yaml:"changesets,omitempty"`
}

type Changeset struct {
	Node    string
	Ref     string
	Comment string
}

type Patch struct {
	Change Changeset
	Sub    *Subscription
}

type Patches map[string][]Patch

type Extra struct {
	Str string
	Mp  map[string]string
}

func (l *Extra) UnmarshalYAML(unmarshal func(interface{}) error) error {
	str := ""
	if err := unmarshal(&str); err == nil {
		*l = Extra{Str: str}
		return nil
	}
	lst := map[string]string{}
	if err := unmarshal(&lst); err == nil {
		*l = Extra{Mp: lst}
		return nil
	}
	return errors.New("Not a map or a string")
}

func (l Extra) MarshalYAML() (interface{}, error) {
	if l.Str != "" {
		return l.Str, nil
	} else if l.Mp != nil {
		return l.Mp, nil
	} else {
		return nil, errors.New("Attempting to Marshal invalid repo")
	}
}

func (l Patch) MarshalYAML() (interface{}, error) {
	if l.Change.Node != "" {
		return l.Change, nil
	} else if l.Sub != nil {
		return l.Sub, nil
	} else {
		return nil, errors.New("Attempting to Marsahal invalid patch")
	}
}

func (l *Patch) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var chg Changeset
	if err := unmarshal(&chg); err == nil {
		*l = Patch{Change: chg}
		return nil
	}
	var sub Subscription
	if err := unmarshal(&sub); err == nil {
		if sub.Branch != "" {
			*l = Patch{Sub: &sub}
			return nil
		}
	}
	return errors.New("Not a string or valid subscription")
}

func (cs *Changeset) UnmarshalYAML(value *yaml.Node) error {
	err := value.Decode(&cs.Node)
	if err != nil {
		return err
	}
	comment := strings.TrimSpace(value.LineComment)
	comment = strings.TrimPrefix(comment, `# `)
	if 0 < len(comment) {
		fields := strings.SplitN(comment, ` `, 2)
		if 0 < len(fields) {
			cs.Ref = fields[0]
		}
		if 1 < len(fields) {
			cs.Comment = fields[1]
		}
	}
	return nil
}

func (cs Changeset) MarshalYAML() (interface{}, error) {
	value := &yaml.Node{}
	err := value.Encode(cs.Node)
	if err != nil {
		return cs.Node, err
	}
	value.LineComment = cs.Ref + ` ` + cs.Comment
	return value, nil
}

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
	delete(remoteBag, `origin`)

	for i, extra := range r.Extras {
		if extra.Str != "" {
			rmtName := fmt.Sprintf(`extra%02d`, i)
			err := SetRemote(gr, rmtName, extra.Str)
			if err != nil {
				return err
			}
			delete(remoteBag, rmtName)
		} else if extra.Mp != nil {
			if extra.Mp["name"] == "" || extra.Mp["path"] == "" {
				return errors.New(`Extra yaml objects require both name and path`)
			}
			err := SetRemote(gr, extra.Mp["name"], extra.Mp["path"])
			if err != nil {
				return err
			}
			delete(remoteBag, extra.Mp["name"])
		} else {
			return errors.New(`Unrecognized type in Extra yaml`)
		}
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

	var newhead []byte
	var err error

	if r.Branch != "" {
		newhead, err = repo.git(`log`, `-n1`, `--format=%H`, path.Join(`origin`, r.Branch))
		if err != nil {
			return errors.New(`Unable to get head of branch origin/` + r.Branch + `: ` + err.Error())
		}
	} else if r.Tag != "" {

		_, err := repo.git(`checkout`, `-B`, r.Tag+`_local`, r.Tag)
		if err != nil {
			return errors.New(`Unable to create branch from Tag ` + r.Tag + `: ` + err.Error())
		}

		newhead, err = repo.git(`log`, `-n1`, `--format=%H`, r.Tag+`_local`)
		if err != nil {
			return errors.New(`Unable to get head of branch ` + r.Tag + `_local` + `: ` + err.Error())
		}
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

const ChangesetGitFormatArg = `--format=%H <%aI!%ae> %s`

/* NewChangeset constructs a new changeset object from a line that matches git format "%H %aI!%ae %s".
 *
 * %aI!%ae is a commit reference as described in the Reposurgeon documentation:
 * http://www.catb.org/esr/reposurgeon/repository-editing.html#_step_six_good_practice
 */
func NewChangeset(line string) Changeset {
	var chg Changeset
	parts := strings.SplitN(line, ` `, 3)
	switch {
	case len(parts) > 2:
		chg.Comment = parts[2]
		fallthrough
	case len(parts) > 1:
		chg.Ref = parts[1]
		fallthrough
	case len(parts) > 0:
		chg.Node = parts[0]
	}
	return chg
}
