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
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/comcast/tsb/loadfiles"
	"gopkg.in/yaml.v3"
)

type Executor struct {
	startDir   string
	configRepo string
	cmds       []string
	at         string
}

type Done struct{}

func (d Done) Error() string { return "Done" }

func (e *Executor) HasArg() bool {
	return len(e.cmds) != 0
}
func (e *Executor) PopArg() string {
	if len(e.cmds) == 0 {
		return ""
	}
	arg := e.cmds[0]
	e.cmds = e.cmds[1:]
	return arg
}

func (e *Executor) Execute() error {
	if !e.HasArg() {
		return Done{}
	}
	if e.startDir == `` {
		e.startDir, _ = os.Getwd()
	}

	cmd := e.PopArg()
	if verbose {
		fmt.Fprintf(os.Stderr, "Performing %s.\n", cmd)
	}
	switch cmd {
	case `fetch`:
		cfg, err := e.Config(e.at)
		if err != nil {
			return err
		}

		return cfg.Repos.Fetch(e.Dir())
	case `build`, `prebuild`:
		cfg, err := e.Config(e.at)
		if err != nil {
			return err
		}

		err = cfg.Repos.Prepare(e.Dir())
		if err != nil {
			return err
		}

		for name, repo := range cfg.Repos {
			if cfg.Patches[name] != nil {
				for _, patch_item := range cfg.Patches[name] {
					chg := patch_item.Change
					if chg.Node != "" {
						err = repo.Cherry(e.Dir(), name, chg.Node)
						if err != nil {
							return fmt.Errorf("Unable to apply %s to %s: "+chg.Node, name, err.Error())
						}
					} else if patch_item.Sub != nil {
						if len(patch_item.Sub.Changesets) > 0 {
							for _, changeset := range patch_item.Sub.Changesets {
								err = repo.Cherry(e.Dir(), name, changeset.Node)
								if err != nil {
									return fmt.Errorf("Unable to apply %s to %s: "+changeset.Node, name, err.Error())
								}
							}
						}
					} else {
						return errors.New(`Unrecognized format in patches yaml file`)
					}
				}
			}
		}

		if cmd == `build` {
			for _, service := range cfg.Compose.ServiceNames() {
				b, err := run(`sudo`, `docker-compose`, `-f`, path.Join(e.Dir(), `docker-compose.yml`), `build`, `--pull`, `--no-cache`, `--force-rm`, service)
				if err != nil {
					return errors.New(`Unable to create build image ` + service + `: ` + err.Error() + "\n" + string(b))
				}

				b, err = run(`sudo`, `docker-compose`, `-f`, path.Join(e.Dir(), `docker-compose.yml`), `run`, `--rm`, service)
				if err != nil {
					return errors.New(`Failed to build ` + service + `: ` + err.Error() + "\n" + string(b))
				}
			}
		}

		return nil
	case `update`:
		if e.at != `` {
			return errors.New("Cannot update from alternate revision.")
		}
		cfg, err := e.Config(``)
		if err != nil {
			return err
		}

		err = cfg.Repos.Update(e.Dir())
		if err != nil {
			return err
		}

		for name, _ := range cfg.Patches {
			g := gitRepo(path.Join(e.Dir(), `src`, name))
			for sub_ind, patch_item := range cfg.Patches[name] {
				if patch_item.Sub != nil {
					b, err := g.git(`log`, `origin/`+cfg.Repos[name].Branch+`..`+patch_item.Sub.Branch, ChangesetGitFormatArg, `--reverse`)
					if err != nil {
						return err
					}
					cslines := strings.Split(string(bytes.TrimSpace(b)), "\n")
					changesets := make([]Changeset, len(cslines))
					for i, csline := range cslines {
						changesets[i] = NewChangeset(csline)
					}
					cfg.Patches[name][sub_ind].Sub.Changesets = changesets
				}
			}
		}

		return e.StoreConfig(cfg)

	case `cherry`:
		arg := e.PopArg()
		if arg == `` {
			return errors.New(`No argument provided to cherry.`)
		}
		if e.at != `` {
			return errors.New("Cannot cherry from alternate revision.")
		}
		repo, changeset := ParseCherry(arg)

		cfg, err := e.Config(``)
		if err != nil {
			return err
		}
		if repo == `` {
			if len(cfg.Repos) != 1 {
				return errors.New("You must supply repository to cherry when you do not have exactly one repository.")
			}
			for repo = range cfg.Repos {
				/* Just use range to get the only repo name available. */
			}
		}

		if _, ok := cfg.Repos[repo]; ok {
			g := gitRepo(path.Join(e.Dir(), `src`, repo))
			b, err := g.git(`log`, `-n1`, ChangesetGitFormatArg, changeset)
			if err != nil {
				return fmt.Errorf(`"%s" is not a valid changeset in "%s": %s`, changeset, repo, err.Error())
			}
			changeset = string(bytes.TrimSpace(b)) /* Set changeset to the output, in case a branch head or tag was passed in. */
		} else {
			return fmt.Errorf(`"%s" is not a valid repository.`, repo)
		}

		new_patch := new(Patch)
		new_patch.Change = NewChangeset(changeset)

		cfg.Patches[repo] = append(cfg.Patches[repo], *new_patch)
		return e.StoreConfig(cfg)

	case `subscribe`:
		arg := e.PopArg()
		if arg == `` {
			return errors.New(`No argument provided to subscribe`)
		}
		if e.at != `` {
			return errors.New(`Cannot subscribe from alternate revision`)
		}
		repo, branch := ParseCherry(arg)

		cfg, err := e.Config(``)
		if err != nil {
			return err
		}

		if repo == `` {
			if len(cfg.Repos) != 1 {
				return errors.New("You must supply repository to subscribe when you do not have exactly one repository.")
			}
			for repo = range cfg.Repos {
			}
		}

		if _, ok := cfg.Repos[repo]; ok {
			g := gitRepo(path.Join(e.Dir(), `src`, repo))
			all_branches, err := g.git(`branch`, `-a`)
			if err != nil {
				return err
			}
			if !strings.Contains(string(all_branches), " remotes/"+string(branch)+"\n") {
				return fmt.Errorf("%s is not a branch in repository %s", branch, repo)
			}
		} else {
			return fmt.Errorf(`"%s" is not a valid repository.`, repo)
		}
		new_sub := new(Subscription)
		new_sub.Branch = branch
		new_patch := new(Patch)
		new_patch.Sub = new_sub
		cfg.Patches[repo] = append(cfg.Patches[repo], *new_patch)
		return e.StoreConfig(cfg)

	case `ls-cherry`:
		return e.ListPatches()
	case `validate`:
		return e.Validate()
	case `patchdiff`:
		return e.PatchDiff()
	case `diff`:
		detailed := true
		return e.Diff(detailed)
	case `changelog`:
		detailed := false
		return e.Diff(detailed)
	case `verbose`, `-v`:
		verbose = true
		return nil
	case `quiet`, `-q`:
		verbose = false
		return nil
	case `cd`:
		/* This isn't generally necessary, but allows the user to
		 * explictly use a directory that matches a command name.
		 */
		cmd = e.PopArg()
		if cmd == `` {
			return errors.New(`No argument provided to cd.`)
		}
		break
	case `at`:
		e.at = e.PopArg()
		return nil
	}

	_, err := os.Stat(path.Join(e.startDir, cmd))
	if err == nil {
		e.configRepo = cmd
		return nil
	}
	return errors.New(`Unknown command ` + cmd)
}

func (e *Executor) Dir() string {
	dir := e.configRepo
	/* Use working directory by default. */
	if dir == `` {
		dir, _ = os.Getwd()
	}
	return dir
}

func (e *Executor) Config(at string) (*Config, error) {
	var cfg Config
	var file loadfiles.File
	if at == `` {
		file = loadfiles.OsFile(e.Dir())
	} else {
		file = loadfiles.GitFile{
			Repo: filepath.Join(e.Dir(), `.git`),
			Rev:  at,
		}
	}
	err := loadfiles.Load(file, &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (e *Executor) StoreConfig(cfg *Config) error {
	return loadfiles.Store(loadfiles.OsFile(e.Dir()), cfg)
}

func (e *Executor) Validate() error {
	cfg, err := e.Config(e.at)
	if err != nil {
		return err
	}
	fmt.Printf("Valid config: %v\n", *cfg)
	return nil
}

func ParseCherry(arg string) (string, string) {
	parts := strings.SplitN(arg, `:`, 2)
	if len(parts) == 1 {
		return ``, arg
	}
	return parts[0], parts[1]
}

func (e *Executor) ListPatches() error {
	cfg, err := e.Config(e.at)
	if err != nil {
		return err
	}

	var repos []string
	for repo := range cfg.Patches {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	for _, repo := range repos {
		fmt.Printf("Patches for %s:\n", repo)
		patch_items := cfg.Patches[repo]
		g := gitRepo(path.Join(e.Dir(), `src`, repo))
		for _, change_item := range patch_items {
			chg := change_item.Change
			if chg.Node != "" {
				b, err := g.git(`log`, `-n1`, `--format=%s (%an) <%aI!%ae>`, chg.Node)
				if err != nil {
					fmt.Printf("\t%s: Failed to find commit; \"%v\"\n", chg.Node, err)
				}
				fmt.Printf("\t%s: %s\n", chg.Node, string(bytes.TrimSpace(b)))
			} else if change_item.Sub != nil {
				fmt.Printf("\tChangesets for subscription to %s:\n", change_item.Sub.Branch)
				if len(change_item.Sub.Changesets) > 0 {
					for _, changeset := range change_item.Sub.Changesets {
						b, err := g.git(`log`, `-n1`, `--format=%s (%an) <%aI!ae>`, changeset.Node)
						if err != nil {
							fmt.Printf("\t\t%s: Failed to find commit; \"%v\"\n", changeset.Node, err)
						} else {
							fmt.Printf("\t\t%s: %s\n", changeset.Node, string(bytes.TrimSpace(b)))
						}
					}
				}
			} else {
				return errors.New("Unrecognized format in patches yaml")
			}
		}
	}
	return nil
}

func (e *Executor) PatchDiff() error {

	g := gitRepo(".")
	b, _ := g.git(`log`, `-n1`, `--format=%s (%an)`)
	fmt.Printf("Differences from previous TSB repo Commit:\n")
	fmt.Printf("\t Commit Message: %s\n", string(b))

	b, err := g.git(`diff`, `HEAD^`, `HEAD`, `--format=%s`, `-U0`, `patches.yml`)
	if err != nil {
		fmt.Printf("err: %s", err)
		return nil
	}

	changes := strings.Split(string(b), "\n")
	var removals []string
	var additions []string
	for _, line := range changes {
		if strings.HasPrefix(line, "-- ") {
			// patch removed
			removals = append(removals, strings.TrimPrefix(line, "-- "))
		}

		if strings.HasPrefix(line, "+- ") {
			// patch added
			additions = append(additions, strings.TrimPrefix(line, "+- "))
		}
	}

	cfg, err := e.Config(e.at)
	if err != nil {
		return err
	}

	var repos []string
	for repo := range cfg.Patches {
		repos = append(repos, repo)
	}
	sort.Strings(repos)

	fmt.Printf("Removals: \n")
	for _, removal := range removals {
		for _, repo := range repos {
			g := gitRepo(path.Join(e.Dir(), `src`, repo))
			msg, err := g.git(`log`, `-n1`, `--format=%s (%an)`, removal)
			if err == nil {
				fmt.Printf("\t%s: %s - %s \n", repo, removal, msg)
				break
			}
		}
	}
	fmt.Printf("Additions: \n")
	for _, addition := range additions {
		for _, repo := range repos {
			g := gitRepo(path.Join(e.Dir(), `src`, repo))
			msg, err := g.git(`log`, `-n1`, `--format=%s (%an)`, addition)
			if err == nil {
				fmt.Printf("\t%s: %s - %s \n", repo, addition, msg)
				break
			}
		}
	}
	fmt.Printf("\n")
	return nil
}

func reverse(arr []Changeset) {
	for ii, jj := 0, len(arr)-1; ii < jj; ii, jj = ii+1, jj-1 {
		arr[ii], arr[jj] = arr[jj], arr[ii]
	}
}

func changesetsFromBytes(buf []byte) (changesets []Changeset) {
	for {
		adv, line, _ := bufio.ScanLines(buf, true)
		if 0 == adv {
			break
		}

		changesets = append(changesets, NewChangeset(string(line)))

		if adv <= len(buf) {
			buf = buf[adv:]
		}
	}
	return changesets
}

// returns changesets new to old for given subdir and hash
func changesetsFor(git gitRepo, hash string) (changesets []Changeset) {
	if buf, err := git.git(`log`, hash, ChangesetGitFormatArg); err == nil {
		changesets = changesetsFromBytes(buf)
	}
	return changesets
}

func patchMapFor(patches []Patch) map[string]Patch {
	var pmap = make(map[string]Patch)
	for _, patch := range patches {
		pmap[patch.Change.Node] = patch
	}
	return pmap
}

type Changelog struct {
	Name           string      `yaml:"name"`
	Repo           string      `yaml:"repo"`
	Head           string      `yaml:"head"`
	Prev           string      `yaml:"prev"`
	CommitsAdded   []Changeset `yaml:"commits_added,omitempty"`
	CommitsRemoved []Changeset `yaml:"commits_removed,omitempty"`
	PatchesAdded   []Changeset `yaml:"patches_added,omitempty"`
	PatchesRemoved []Changeset `yaml:"patches_removed,omitempty"`
}

type Changelogs []Changelog

func (l Changelogs) PrintMarkdown(detailed bool) {
	first := true
	for _, change := range l {
		if first {
			first = false
		} else {
			fmt.Println()
		}

		title := change.Name + ` ` + change.Repo
		fmt.Println(title)
		fmt.Println(strings.Repeat(`=`, len(title)))

		fmt.Println(`- HEAD:`, change.Head)
		fmt.Println(`- Prev:`, change.Prev)

		var pfunc = func(title string, changes []Changeset, detailed bool) {
			fmt.Println()
			fmt.Println(title)
			fmt.Println(strings.Repeat(`-`, len(title)))
			for _, change := range changes {
				if detailed {
					fmt.Println(`-`, change.Node, change.Ref, change.Comment)
				} else {
					// did someone hand edit this file?
					if 0 < len(change.Comment) {
						fmt.Println("-", change.Comment)
					} else {
						fmt.Println("-", change.Node)
					}
				}
			}
		}

		if 0 < len(change.CommitsAdded) {
			pfunc(`Commits Added`, change.CommitsAdded, detailed)
		}

		if 0 < len(change.CommitsRemoved) {
			pfunc(`Commits Removed`, change.CommitsRemoved, detailed)
		}

		if 0 < len(change.PatchesAdded) {
			pfunc(`Patches Added`, change.PatchesAdded, detailed)
		}

		if 0 < len(change.PatchesRemoved) {
			pfunc(`Patches Removed`, change.PatchesRemoved, detailed)
		}
	}
}

func (e *Executor) Diff(detailed bool) error {

	var changelogs Changelogs

	var tsblog Changelog
	tsblog.Name = `tsb`

	git := gitRepo(`.`)

	// TSB diffs
	hashbytes, err := git.git(`rev-parse`, `HEAD`)
	if err != nil {
		return err
	}
	tsblog.Head = string(bytes.TrimSpace(hashbytes))

	tsblog.Prev = e.PopArg()
	if tsblog.Prev == `` {
		hashbytes, err := git.git(`rev-parse`, `HEAD^1`)
		if err != nil {
			return err
		}
		tsblog.Prev = string(bytes.TrimSpace(hashbytes))
	}

	remotebytes, _ := git.git(`remote`, `get-url`, `origin`)
	tsblog.Repo = string(bytes.TrimSpace(remotebytes))

	tsbbytes, _ := git.git(`log`, ChangesetGitFormatArg, tsblog.Prev+`..HEAD`)
	tsblog.CommitsAdded = changesetsFromBytes(tsbbytes)

	changelogs = append(changelogs, tsblog)

	// Parse repos and patches
	rbyteshead, _ := git.git(`show`, `:repos.yml`)
	rbytes, _ := git.git(`show`, tsblog.Prev+`:repos.yml`)

	pbyteshead, _ := git.git(`show`, `:patches.yml`)
	pbytes, _ := git.git(`show`, tsblog.Prev+`:patches.yml`)

	// repository
	var reposhead Repos
	yaml.Unmarshal(rbyteshead, &reposhead)
	var repos Repos
	yaml.Unmarshal(rbytes, &repos)

	// cherry picked patches
	var patcheshead Patches
	yaml.Unmarshal(pbyteshead, &patcheshead)
	var patches Patches
	yaml.Unmarshal(pbytes, &patches)

	for key, repohead := range reposhead {

		var changelog Changelog
		changelog.Name = key

		git := gitRepo(`src/` + key)
		remotebytes, _ := git.git(`remote`, `get-url`, `origin`)
		changelog.Repo = string(bytes.TrimSpace(remotebytes))

		changesetshead := changesetsFor(git, repohead.Head)
		// ensure changesets are old to new
		reverse(changesetshead)

		repo, ok := repos[key]
		if !ok {
			fmt.Println("No repo found for:", key)
			continue
		}
		changesets := changesetsFor(git, repo.Head)
		// ensure changesets are old to new
		reverse(changesets)

		index := 0
		for index < len(changesetshead) && index < len(changesets) {
			if changesetshead[index].Node != changesets[index].Node {
				break
			}
			index++
		}

		changelog.Head = repohead.Head
		changelog.Prev = repo.Head

		if verbose {
			fmt.Println("ChangesetsHEAD:", len(changesetshead))
			fmt.Println("Changesets:", len(changesets))
			fmt.Println("First diff index:", index)
		}

		// Trim changesets to just include deviation points
		changesetshead = changesetshead[index:]
		// change back to new to old
		reverse(changesetshead)
		changelog.CommitsAdded = changesetshead

		changesets = changesets[index:]
		// change back to new to old
		reverse(changesets)
		changelog.CommitsRemoved = changesets

		pmaphead := patchMapFor(patcheshead[key])
		pmap := patchMapFor(patches[key])

		if verbose {
			fmt.Println()
			fmt.Println("PatchesHEAD:", len(pmaphead))
			fmt.Println("Patches:", len(pmap))
		}

		for key, patch := range pmap {
			if _, found := pmaphead[key]; !found {
				changelog.PatchesRemoved = append(changelog.PatchesRemoved, patch.Change)
			}
		}

		for key, patch := range pmaphead {
			if _, found := pmap[key]; !found {
				changelog.PatchesAdded = append(changelog.PatchesAdded, patch.Change)
			}
		}

		changelogs = append(changelogs, changelog)
	}

	changelogs.PrintMarkdown(detailed)

	return nil
}
