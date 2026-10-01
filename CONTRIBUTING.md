# Contributing

Thanks for taking the time. Issues and pull requests are welcome, for anything larger
than a fix please open an issue first, so the idea can be discussed before the code.

## Development

```sh
pre-commit install  # Once: check every commit, its message, and every push.
make check          # Format, lint, tidy and test, without modifying anything.
make fix            # Apply every automatic fix.
make help           # List every target.
```

CI runs the same checks on every pull request, against the minimum Go version in
`go.mod` and the latest release.

## Conventions

**Comments** are full sentences that explain why, not what. They do not use dashes as
punctuation or chains of semicolons, a colon or a new sentence reads better.

**Event ids** of go-see itself, everything under `see`, are a stable contract. Never
rename or repurpose one, add a new id instead.

**Dependencies** are kept to zap and the standard library. A new one needs a very good
reason.

**Commits** follow [Conventional Commits](https://www.conventionalcommits.org), e.g.
`feat(emit): add trace correlation` or `fix(id): reject empty segments`. Pull requests
are squash merged, so it is the pull request title that has to follow them, CI checks it.

## Releases

Releases are cut by [semantic-release](https://github.com/semantic-release/semantic-release)
on every push to `main`, once CI is green. It reads the commits since the last tag,
pushes the next `vX.Y.Z` tag, and publishes the release notes as a
[GitHub release](https://github.com/TimCares/go-see/releases). Nobody tags by hand.
Finally, it requests the new version from `proxy.golang.org`, which makes it show up on
[pkg.go.dev](https://pkg.go.dev/github.com/TimCares/go-see) right away.

| Commit                                      | Release                  |
| ------------------------------------------- | ------------------------ |
| `fix: ...`, `perf: ...`                     | Patch                    |
| `feat: ...`                                 | Minor                    |
| `feat!: ...`, or a `BREAKING CHANGE` footer | Minor, while pre `v1`    |
| `docs: ...`, `chore: ...`, `ci: ...`, ...   | None                     |

While go-see is pre `v1`, breaking changes bump the minor version, as Go treats
`v0` as unstable anyway. Going `v1` means removing that rule from `.releaserc.json`.
