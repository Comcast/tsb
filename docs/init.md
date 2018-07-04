Getting Started with `tsb`
==========================

*This guide assumes that you're familiar with `git`, Docker, and
`docker-compose`, though not necessarily an expert.*

Let's assume that you want to use `tsb` to build a hypothetical tool called
Widget, hosted at https://git.example.net/widget. It builds with a simple
`make` which puts it's binary, `do-widget`, in the current directory.

Repository Setup
----------------

We'll set up each of the files one at a time. You'll want a new directory, so
start out with a `git init tsb-widget && cd tsb-widget`. Everything else will
assume `tsb-widget` as the current directory unless noted otherwise. `tsb`
expects to operate on its current directory (although it can be instructed to
work elsewhere).

Each of these files is documented in the [README](../README.md), so check that
out for more details.

### `tsb`

The zeroth step is to get `tsb` itself. `tsb` is distributed as source, so
you'll need a working [go](https://golang.org/dl) install.

    go get github.com/comcast/tsb

That should install `tsb` in your `$GOPATH/bin` folder. Either add
`$GOPATH/bin` to your path, symlink to it from somewhere in your path, or `sudo
install` it into your path.

### `repos.yml`

The first real step is telling `tsb` where to find the source. `tsb` only
operates on `git` repos (for now!), so you'll need to offer a path to a git
repository with the code. In our case, this path is
`https://git.example.net/widget`.

The file itself is [YAML v1.2](http://yaml.org) and should look something like:

```yaml
wgt: # A friendly name for the repo, will be used for the dir name
  src: https://git.example.net/widget
  branch: master
```

There are a few extra fields, but `tsb` can fill them in for you. If you wanted
to "track" a different branch (perhaps a release branch), you'd put that here
instead of `master`.

It's worth mentioning, the `branch` here isn't always the head to build, but
rather where we want to update to when we do `tsb update`. So let's go ahead
and do that now:

    tsb fetch update

That will go get a copy of the source (which is necessary if we want to know
what things look like in order to update) and then update the repos file to
match the branch target. Open the `repos.yml` file and you'll see something
that looks like:

```yaml
wgt:
  src: https://git.example.net/widget
  branch: master
  head: <long changeset id here>
  extra: []
```

The `head` field now exists and points to the head of master. This is because
we always want a one-to-one match between our `tsb` config and a resultant
binary.

When you `build`, you use `head`. When you `update`, you use `branch`.

### `docker-compose.yml`

The next file to make is the compose file. The key thing to remember for this
file is that the "input" is at `./src/wgt` and the output is at `./dist`. (The
directory name is the name you chose for the key in the repos.yml.) The compose
file might look like this:

```yaml
version: "3"
services:
  wgt_build:
    image: wgt_build
    build:
      dockerfile: Dockerfile
      context: .
    volumes:
    - ./src:/opt/src
    - ./dist:/opt/dist
```

This just mounts the input at `/src` and the output at `/dist`. It will be up
to the Dockerfile to make sure that happens.

### `Dockerfile`

So we need a simple Dockerfile to actually run the build. The exact details
will depend heavily on your exact product. But for the example, lets suppose we
need a few tools:

```dockerfile
FROM centos:7
RUN yum clean all && yum install -y \
    gcc \
    make \
    git
CMD make -C /opt/src/wgt && cp /opt/src/wgt/widget /opt/dist/
```

In practice, anything more than a trivial build will probably want a separate
script that you `ADD` to the container and then run it as the `CMD`.

A keen observer will note that we just broke our promise of one-to-one
compatibility. After all, this will install the latest `gcc`, which might have
a bug that breaks the resultant binary. Exactly how careful you are to avoid
this is up to you. You can, if you wish, pin the exact version for the up-line
image and every package you install. If you really want, you can bundle the
install packages inside the `tsb` configuration dir and `ADD` them to the
Dockerfile. Your needs will dictate the approach.

### `patches.yml`

If you're just starting out, this file should probably be empty! You can just
`touch patches.yml` to get started.

### Putting it together

    tsb build

You should be able to build the project now. The output will wind up in the
`./dist` directory, exactly as expected. There's a good chance that something
went wrong, however. You can get more information by passing the `verbose`
command: `tsb verbose build`.

Once you've got a successful first build, commit your `tsb` config files:

```bash
git add repos.yml patches.yml docker-compose.yml Dockerfile
git commit -m 'Initial commit.'
```

From here on out, remember that the `tsb` config repo is just a git repo like
any other, so you can use all the same tools: PRs, branches, history, blame, et
cetera.

Working with `tsb`
------------------

Once you've got a build, you'll want to be able to work with it. Here are some
common tasks...

### Add a patch from a local repository

Eventually, it's likely that you'll want to add a patch that isn't upstream
yet. Using our hypothetical widget repo from before, let's say the coding for
the patch looks like this:

```bash
cd ~/widget
git checkout master
git checkout -b electron-notice
echo "Made with 100% Artisanal Electrons" >> README.md
git add README.md
git commit -m 'Added required notice regarding electron content.'
```

Neat, now the repo has a branch with the required fix, but let's suppose it's
not quite ready for the upstream community. We still need a build, though.

When we did `git commit`, it printed out the new changeset id. We'll want that
later, or we can use `git log` to retrieve it when we need it.

Create a fork somewhere nearby, in either a public or a private repository
management system. Let's suppose for the example that this fix is private,
within an Enterprise: https://git.enterprisey.example.com/widget.

Push the branch to the newly created internal fork:

```bash
git remote add ds https://git.enterprisey.example.com/widget
git push ds electron-notice
```

Add the repository to the `extras` in `repos.yml`:

```yaml
wgt:
  src: https://git.example.net/widget
  branch: master
  head: <long changeset id here>
  extra:
  - https://git.enterprisey.example.com/widget
```

This will let `tsb` know to pull changesets from there as well, which you'll
want to do right away:

    tsb fetch

Add the changeset itself:

    tsb cherry <git id from above>

You can then `tsb build` the result. Don't forget to `git commit` the changed
tsb config files.

From now on, you don't need to add the `extra`, you can just push to the
internal repo and `tsb fetch cherry <id>`.

### List the patches

Since `patches.yml` doesn't include descriptions, you can use `tsb ls-cherry`
to get commit summaries for each patch.

### Remove a patch

Just edit `patches.yml` to remove the relevant changeset. You can use `tsb
ls-cherry` to figure out which line needs to be removed.

### Fix a patch that didn't apply

First, check to see if the patch didn't apply because it's empty. This often
happens when a patch you've been carrying for a while is accepted and merged in
upstream. The fix here is easy... remove the changeset from `patches.yml`.

Otherwise, you'll want to figure out what you changed to conflict things. If
you most recently did `tsb update` and now one or more patches are failing to
apply, you'll want to rebase them.

Let's suppose our patch from the example in "Add a patch from a local
repository" is breaking:

```bash
cd ~
git clone https://git.enterprisey.example.com/widget enterprisey-widget
cd enterprisey-widget
git checkout electron-notice
git rebase master
# Pause and resolve conflicts. There will be conflicts.
git rebase --continue
git push -f electron-notice
NEWID=$(git log electron-notice...master --format=%H) # ... or just copy-paste from
                                                      # one of the commands above
                                                      # that prints out the new id.
cd ~/tsb-widget
vi patches.yml # Remove old changeset.
tsb fetch cherry $NEWID # ... or paste from before.
```

If, on the other hand, you are adding a new patch and it fails to apply, there
are two possibilities. If it's failing because it conflicts with something on
the head, follow the previous advice. Otherwise, you'll need to cherry-pick it
to after the changeset that comes before it in `patches.yml` and, naturally,
fix the conflicts.
