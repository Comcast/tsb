Transitive Source Theory
========================

At a basic level, the problem to be solved here is this:

> Upstream and downstream projects travel at different speeds.

There are a few techniques for handling this, but the basic process is...

1. Wait for a convenient time to "incorporate upstream";
2. Pull in upstream changes;
3. Add in anything that hasn't been upstreamed;
4. Produce an internal build; and
5. Repeat.

Approaches
----------

There's nothing new about this approach. It's nearly as old as the idea of
source code and there are lots of ways to manage it.

### Specfile Patches

Customizing Specfiles is an implementation often used by RPM-based maintainers.
Specfiles often have patches:

    Patch0: widget-fix-foo.patch
    Patch1: widget-add-private.patch

    # ... and later, at the end of the the %prep section
    %patch0 -p0
    %patch1 -p1

This method is time-tested, source-control neutral, and very well understood.
It only works well, however, for building RPMs from tarballs and similar
sources.

### Continuous Rebase

    git checkout master
    git pull upstream
    git checkout ds-branch
    git rebase master
    git push -f downstream ds-branch

This approach also works somewhat reasonably. Instead of managing patches, you
manage changesets and continually rebase them against the appropriate upstream
branch. This requires a Distributed Version Control System, like git or
Mercurial, but can be used for any kind of project.

When developers build against this branch, though, their "base" will be
continuously changing out from under them. The [git
manual](https://git-scm.com/docs/git-rebase) says:

> Rebasing (or any other form of rewriting) a branch that others have based
> work on is a bad idea: anyone downstream of it is forced to manually fix
> their history.

This makes developing against the downstream quite tedious.

### Dynamic Construction

This is the approach that `tsb` uses. It takes the flexibility and severability
of Specfile Patches, but uses git to manage them. It looks a bit more like this:

    git checkout master
    git pull upstream
    git checkout -b tmp-branch
    for cs in $(cat patch-list); do
        git cherry-pick $cs
    done
    make
    git checkout master
    git branch -D tmp-branch

The big thing to note is that in the example, `tmp-branch` doesn't persist. And
rather than a private branch being rebased, it's simply a list of changesets to
patch in.

At it's simplest, this is very much like continuous rebase. In fact, replace
`cat patch-list` with a list of changesets in a branch and it amounts to
precisely the same thing.

But since there's no permanent branch, developers aren't tempted to build work
on top of it. Instead, they generally build as a branch of the upstream head.
This provides better stability for development.

At build time, everything is dynamically constructed, built, and then cleaned
up. The dynamic nature means that sometimes, you can wind up with build failures
caused by failed cherry-picks. Fortunately, these are pretty easy to resolve:

 - **A revision in the patch list conflicts with changes pulled in from
   upstream.** Rebase/cherry-pick your revision onto the latest from upstream.
   Fix the conflicts, commit it and store the resolved version. Update the
   patch list to use the resolved revision instead.
 - **A revision in the patch list conflicts with changes from a different
   revision in the patch list.** Rebase/cherry-pick the second revision onto
   the first. Fix the conflicts, commit, and point the patch list at the new
   version.
 - **A revision in the patch list does not apply because it is now empty.**
   Remove it from the patch list, it has been incorporated upstream now.

Goals
-----

Downstream maintainers need to do several things:

 - Incorporate upstream changes
 - Add new patches
 - Send patches upstream
 - Remove patches
 - List how downstream differs from upstream

If your source base is constructed dynamically, like with the Specfile Patches
or the Dynamic Construction, adding and removing patches is easy. If your
downstream strategy uses DVCS to manage changes, it's easy to incorporate the
upstream changes, add new patches, and send patches back upstream.

It's easy to forget how important it is to remove patches. When a fix has gone
upstream, or a local hack is no longer necessary, or when an experimental
change is abandoned, those changes need to disappear. If they don't, then it's
very hard to reliably list what changes are present in the downstream
repository that are not in the upstream.

`tsb`
-----

`tsb` attempts to make it as easy as possible to accomplish those goals quickly
and easily, but anyone with excellent facility using a DVCS can accomplish them
as well. It supports multiple repositories and manages builds and dependencies
using Docker.
