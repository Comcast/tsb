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

/* This will provide version diff functionality, when completed.
 * The idea is to take a two changeset ids of the tsb repo and produce
 * a list of changesets that differ between them.
 */

/*

import (
	"errors"
	"time"
)

type Changeset struct {
	Repo        gitRepo
	Author      string
	Description string
	Time        time.Time
	Hash        string
}
type Changesets []Changeset

func NewChangeset(repo gitRepo, hash string) (Changeset, error) {
	getField := func(field string) (string, error) {
		data, err := repo.git(`log`, `--pretty=%`+field, hash)
		return string(data), errors.New("Unable to retrieve field " + field + ": " + err.Error())
	}

	var err Error
	cs := Changeset{
		Repo: repo,
		Hash: hash,
	}
	cs.Author, err = getField(`aE`)
	if err != nil {
		return cs, err
	}

	cs.Description, err = getField(`B`)
	if err != nil {
		return cs, err
	}

	t, err := getField(`ai`)
	if err != nil {
		return cs, err
	}
	cs.Time, err = time.Parse(t, `2006-01-02 15:04:05 -0700`)
	if err != nil {
		return cs, errors.New("Unable to parse authorship time " + t + ": " + err.Error())
	}

	return cs, nil
}

type ChangesetDiff struct {
	Changeset
	Action DiffAction
}

type DiffAction string

const (
	DiffAdd    = "Added"
	DiffRemove = "Removed"
	DiffMove   = "Moved"
)

func GetChangesets(repo gitRepo, include string, exclude string) (Changesets, error) {
	var css Changesets

}
*/
