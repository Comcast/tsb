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
	"sort"
	"strings"

	"github.com/comcast/tsb/loadfiles"
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
					if patch_item.Str != "" {
						err = repo.Cherry(e.Dir(), name, patch_item.Str)
						if err != nil {
							return fmt.Errorf("Unable to apply %s to %s: "+patch_item.Str, name, err.Error())
						}
					} else if patch_item.Sub != nil {
						if len(patch_item.Sub.Changesets) > 0 {
							for _, changeset := range patch_item.Sub.Changesets {
								err = repo.Cherry(e.Dir(), name, changeset)
								if err != nil {
									return fmt.Errorf("Unable to apply %s to %s: "+changeset, name, err.Error())
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
					b, err := g.git(`log`, `origin/`+cfg.Repos[name].Branch+`..`+patch_item.Sub.Branch, `--format=%H`, `--reverse`)
					if err != nil {
						return err
					}
					changesets := strings.Split(string(bytes.TrimSpace(b)), "\n")
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
			}
		}

		if _, ok := cfg.Repos[repo]; ok {
			g := gitRepo(path.Join(e.Dir(), `src`, repo))
			b, err := g.git(`log`, `-n1`, `--format=%H`, changeset)
			if err != nil {
				return fmt.Errorf(`"%s" is not a valid changeset in "%s": %s`, changeset, repo, err.Error())
			}
			changeset = string(bytes.TrimSpace(b)) /* Set changeset to the output, in case a branch head or tag was passed in. */
		} else {
			return fmt.Errorf(`"%s" is not a valid repository.`, repo)
		}

		new_patch := new(Patch)
		new_patch.Str = changeset

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
			if change_item.Str != "" {
				b, err := g.git(`log`, `-n1`, `--format=%s (%an)`, change_item.Str)
				if err != nil {
					fmt.Printf("\t%s: Failed to find commit; \"%v\"\n", change_item.Str, err)
				}
				fmt.Printf("\t%s: %s\n", change_item.Str, strings.TrimSpace(string(b)))
			} else if change_item.Sub != nil {
				fmt.Printf("\tChangesets for subscription to %s:\n", change_item.Sub.Branch)
				if len(change_item.Sub.Changesets) > 0 {
					for _, changeset := range change_item.Sub.Changesets {
						b, err := g.git(`log`, `-n1`, `--format=%s (%an)`, changeset)
						if err != nil {
							fmt.Printf("\t\t%s: Failed to find commit; \"%v\"\n", changeset, err)
						} else {
							fmt.Printf("\t\t%s: %s\n", changeset, strings.TrimSpace(string(b)))
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
