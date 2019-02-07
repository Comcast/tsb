tsb
===

`tsb` is the Transitive Source Builder.

It is designed to allow organizations, teams, and individuals to manage
their builds of systems that may be shared with others.

There is a [quickstart guide](docs/init.md) to getting going with `tsb` as well
as a [strategy guide](docs/theory.md) for managing downstream repositories in
general.

Usage: `tsb [options] [commands]`

Commands
--------
  - `tsb fetch` acquires all the repositories for `/src/`.
  - `tsb build` builds the build branch, with patches applied.
  - `tsb update` fetches the latest updates and creates a new commit in
    the config repository. This will also fetch the latest updates in the 
    subscribed branches and update the patch file's subscriptions.
  - `tsb cherry {hash}` cherry-picks `{hash}` and adds it to the patch
    file.
  - `tsb subscribe {branch}` subscribes to the given branch. The branch must
    be in the form `{repoName}:{branchName}`. The branch name must specify
    the name of the remote that the subscription should be pulled from (`remoteName/branchName`). 
    The subscription is then added to the patch file.
  - `tsb ls-cherry` lists out the current list of cherry-picks, along
    with some basic information about them to help identify them.
  - `tsb verbose` and `tsb quiet` do nothing on their own, but set the
    output to be verbose and quiet, respectively. `-v` and `-q` are
    synonyms.
  - `tsb cd {dir}` does the same as `tsb {dir}`, except that it always
    cds, even if {dir} matches the name of a command.
  - `tsb at {rev}` causes commands that follow to pull data from that
    revision in source control. This functionality requires that the
    `tsb` directory be a git repository.
  - `tsb {dir}` changes directory into `{dir}`. This is useful for
    running `tsb` against a subdirectory.

Config Repository
-----------------

`tsb` operates on a repository, not just a config file. The repository
should have three `.yml` files at the root:

    /docker-compose.yml
    /patches.yml
    /repos.yml

During builds, the source for each repository will be checked out to:

    /src/{reponame}

where `{reponame}` is the name of that repository in the repos file. The
`docker-compose.yml` file will need to refer to these repositories or
their contents by this path.

Builds will be expected to produce a list of artefacts in:

    /dist/

The repository will require additional files to support those files

### docker-compose.yml

The compose file may define any number of targets with any names, they
will all be run as part of the build process. One or more dockerfiles
may be referenced from here.

Source repositories will be in predictable locations as noted above
(`/src/{reponame}`), so they can and should be mounted as volumes in the
container. Build tasks should generally not modify the volumes directly,
but rather copy them to a working directory if they might be modified
by build tools.

Likewise, `/dist/` should generally be mounted so that output files can
be put there.

#### Dockerfile

The dockerfile (referenced by the docker-compose.yml) should define the
build environment. Generally, it should add a script that serves as the
entrypoint for the build. It should not run the build directly via `RUN`
instructions, since build states may be cached by docker.

The run script should copy artefacts and log files to the location in
the container where `/dist/` is mounted and return 0 iff the build was
successful.

### patches.yml

`patches.yml` is a YAML 1.2 file. It should be a list of whose members are hashes
or branch subscriptions:

    superwidget:
      changesets:
      - {changeset1}
      - branch: beta
        changesets:
          - {changesetA}
          - {changesetB}
      - {changeset2}

The hashes can refer to any changeset in any of the referenced sources.
Each changeset will be cherry-picked, in order, to the head of the branch
to be built.

The `patches.yml` file can be empty. Indeed, empty is the most desirable
state, since that means building directly against the primary
repository.

This file will not usually need to be modified manually. The `tsb cherry`,
`tsb subscribe` and `tsb update` commands will safely modify this file.

### repos.yml

`repos.yml` is a YAML 1.2 file. It should be an object with a member for
each repository:

    superwidget:
      src: https://upstream.example.net/repos/superwidget
      branch: master
      head: 4817590950ca0b52d3336011a1abdbb6f906e23228c5857cc0f7703828f6966f
      extra:
        - https://private.example.com/repos/superwidget-alpha
        - path: https://private.example.com/repos/superwidget-beta
          name: beta
    hyperwidget:
      src: https://upstream.example.net/repos/hyperwidget
      branch: lts-7.2

Each repository member object should have:
  - a `src` member, which provides the address whence to fetch the
    source repository;
  - a `branch` member, which defines the branch being tracked for
    builds;
  - a `head` member, which is an explicit changeset hash to build; and
  - an optional `extra` member, which is a list of extra source
    addresses that should be fetched in addition to the primary `src`.
    These list members can either be a string representation of the path
    or an object containing a `name` and a `path`. If a name is provided,
    the remote will be given that name when added.

When building, `branch` is ignored; `head` controls. `branch` is used to
update `head` with `tsb update`.
