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

import "sort"

type Compose struct {
	Version  string          `yaml:"version"`
	Services ComposeServices `yaml:"services"`
}

type ComposeServices map[string]ComposeService

type ComposeService struct {
	Image   string              `yaml:"image"`
	Build   ComposeServiceBuild `yaml:"build"`
	Volumes []string            `yaml:"volumes"`
}

type ComposeServiceBuild struct {
	Dockerfile string `yaml:"dockerfile"`
	Context    string `yaml:"context"`
}

func (c Compose) ServiceNames() []string {
	var names []string
	for name := range c.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
