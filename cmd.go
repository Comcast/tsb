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

	"github.comcast.com/cdneng/tsb/loadfiles"
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
	case `build`:
		cfg, err := e.Config(e.at)
		if err != nil {
			return err
		}

		err = cfg.Repos.Prepare(e.Dir())
		if err != nil {
			return err
		}
		for name, repo := range cfg.Repos {
			for _, cherry := range cfg.Patches[name] {
				err = repo.Cherry(e.Dir(), name, cherry)
				if err != nil {
					return fmt.Errorf("Unable to apply %s to %s: "+cherry, name, err.Error())
				}
			}
		}

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

		cfg.Patches[repo] = append(cfg.Patches[repo], changeset)
		return e.StoreConfig(cfg)
	case `ls-cherry`:
		return e.ListPatches()
	case `validate`:
		return e.Validate()
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
		patches := cfg.Patches[repo]
		if len(patches) > 0 {
			fmt.Printf("Patches for %s:\n", repo)

			g := gitRepo(path.Join(e.Dir(), `src`, repo))
			for _, patch := range patches {
				b, err := g.git(`log`, `-n1`, `--format=%s (%an)`, patch)
				if err != nil {
					fmt.Printf("\t%s: Failed to find commit; \"%v\"\n", patch, err)
				} else {
					fmt.Printf("\t%s: %s\n", patch, strings.TrimSpace(string(b)))
				}
			}
		}
	}
	return nil
}
