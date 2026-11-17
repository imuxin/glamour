# Philo maintenance fork

This fork maintains Philo's terminal Markdown link behavior on top of
https://github.com/charmbracelet/glamour. The module path stays
`charm.land/glamour/v2`; consumers use a versioned Go `replace` to
`github.com/imuxin/glamour/v2`.

## Branches and releases

- `philo` is the default branch and carries the downstream patches and automation.
- `main` is the original upstream snapshot; automation does not publish to it.
- The migration baseline is upstream `010543cac1a07c9bfd9969e58d683f2ddef4c8f7`.
- Consumer release tags (`v2.0.2-philo.1`, etc.) are immutable. Never move or delete
  a published release tag. Rebasing `philo` does not upgrade an application pin.
- Before each automated rebase is published, the old head is retained under
  `philo-history/<full SHA>`. The backup and branch update are pushed atomically.

## Downstream changes

Use upstream `ansi.HyperlinkModeInline` / `glamour.WithHyperlinkMode`; the previous
private `HideLinkURL` option is not carried forward.

The patches add empty-description fallback, preserve nested emphasis and literal
inline code, and render regular links in table cells without address footnotes.
Bare URLs, email autolinks, and standalone images retain their existing behavior.
The added regression tests live in `ansi/link_hide_url_test.go`.

Changes affect `ansi/link.go`, `codespan.go`, `elements.go`, `emphasis.go`,
`table.go`, and `table_links.go`. Keep patches focused and remove them when
upstream provides equivalent behavior. Preserve upstream tests and the MIT
license. Philo-specific theme, viewport, and clipboard behavior belongs in the
Philo repository.

## Automated upstream synchronization

`Philo upstream sync` runs daily at 03:23 UTC (11:23 Asia/Shanghai) and supports
manual dispatch. GitHub schedules may be delayed, and GitHub can disable schedules
on inactive public forks after 60 days; check Actions if updates stop.

The workflow fetches upstream `main`, rebases `philo`, runs `go test -race ./...`
and `go vet ./...`, and only then pushes with an explicit `--force-with-lease`.
Conflicts and failed checks leave the remote branch unchanged. Inspect the failed
Actions run and summary; the workflow does not create issues or send custom
notifications. GitHub's own workflow notification settings still apply.

No PAT or custom secret is required: the workflow uses `GITHUB_TOKEN` with
`contents: write`. Only the synchronization workflow needs write permission.
Inherited upstream release/coverage/config-sync workflows are disabled in this
fork's Actions settings because they require upstream services or credentials.
The fork's own CI tests normal pushes and pull requests. The sync job tests its
own rebased commit because pushes made with `GITHUB_TOKEN` do not trigger another
push workflow.

## Resolving a failed synchronization

Use a clean checkout and fetch both remotes:

```sh
git fetch origin philo
git fetch upstream main
git switch -c resolve-upstream origin/philo
old_head=$(git rev-parse HEAD)
git rebase upstream/main
# Resolve conflicts, git add the resolved files, then git rebase --continue.
go test -race ./... -timeout=120s
go vet ./...
git tag "philo-history/$old_head" "$old_head"
git push --atomic --force-with-lease="refs/heads/philo:$old_head" origin \
  HEAD:refs/heads/philo "refs/tags/philo-history/$old_head"
```

Do not resolve conflicts by discarding downstream tests or automatically choosing
one side. After an automated rebase, local work should start from the newly fetched
`origin/philo`; cherry-pick any unpublished local commits rather than merging the
old and new histories.

## Updating Philo

After validating a candidate in the Philo workspace, create a new immutable
`v2.x.y-philo.N` tag at that candidate and push just that tag. Set Philo's module
replacement to that release, refresh checksums, and run its integration tests.
The fork workflow deliberately does not modify Philo or advance its dependency
pin. A rollback changes the consumer pin to an earlier tag.
