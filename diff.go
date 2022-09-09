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
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
			if 0 < len(changes) {
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
		}

		pfunc(`Commits Added`, change.CommitsAdded, detailed)
		pfunc(`Commits Removed`, change.CommitsRemoved, detailed)
		pfunc(`Patches Added`, change.PatchesAdded, detailed)
		pfunc(`Patches Removed`, change.PatchesRemoved, detailed)
	}
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

func Diff(prevhash string, detailed bool) error {

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

	if prevhash == `` {
		hashbytes, err := git.git(`rev-parse`, `HEAD^1`)
		if err != nil {
			return err
		}
		prevhash = string(bytes.TrimSpace(hashbytes))
	}

	tsblog.Prev = prevhash

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

		changelog.Head = repohead.Head

		// Look up repo
		repo, ok := repos[key]
		if !ok {
			fmt.Println("No repo found for:", key)
			changelog.Prev = `<none>`
			changelogs = append(changelogs, changelog)
			continue
		}

		delete(repos, key)

		changelog.Prev = repo.Head

		changesetshead := changesetsFor(git, repohead.Head)
		// ensure changesets are old to new
		reverse(changesetshead)

		changesets := changesetsFor(git, repo.Head)
		// ensure changesets are old to new
		reverse(changesets)

		// Find first index not in common between the 2 changesets
		index := 0
		for index < len(changesetshead) && index < len(changesets) {
			if changesetshead[index].Node != changesets[index].Node {
				break
			}
			index++
		}

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

	// prev repos not in HEAD
	for key, repo := range repos {

		var changelog Changelog
		changelog.Name = key
		changelog.Repo = `<unknown>`
		changelog.Head = `<none>`
		changelog.Prev = repo.Head

		changelogs = append(changelogs, changelog)
	}

	changelogs.PrintMarkdown(detailed)

	return nil
}
