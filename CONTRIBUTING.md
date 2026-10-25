# Contributing to grepai

Thank you for wanting to help. grepai works **issue first**: every contribution starts as an issue, and the project does not accept pull requests.

## Why issue first

With AI coding tools, writing a fix is rarely the hard part. The hard part is knowing exactly what is wrong, on which setup, and what the right behavior is. A precise issue carries all of that. Reviewing an outside pull request, checking it does not break other setups, and reworking it to fit the rest of the code usually takes longer than implementing the fix from a good issue.

So the most useful thing you can send is a detailed issue. The maintainer implements it, tests it against the current release on real projects, and credits you in the changelog.

Pull requests opened anyway are closed with a link to open an issue instead. Nothing is lost: the description and the diff you wrote help write the fix.

## Open an issue

- **Bug**: use the [bug report form](https://github.com/yoanbernabeu/grepai/issues/new?template=bug_report.yml). Include the grepai version (`grepai version`), your OS, the embedder and store from `.grepai/config.yaml` (without keys), the exact commands, what you expected, what happened, and the logs.
- **Feature**: use the [feature request form](https://github.com/yoanbernabeu/grepai/issues/new?template=feature_request.yml). Start with the problem you hit and your real use case before the solution you have in mind.
- **Question**: use [Discussions](https://github.com/yoanbernabeu/grepai/discussions).
- **Security issue**: do not open a public issue; follow [SECURITY.md](SECURITY.md).

Search the [existing issues](https://github.com/yoanbernabeu/grepai/issues?q=is%3Aissue) first. If yours is there, add your setup and logs to it: a second reproduction is often what unblocks a fix.

## Other ways to help

- **Test a fix.** When a fix lands on a branch, building it and confirming it works on your project is very valuable. See [Building from source](#building-from-source).
- **Improve the docs.** If a page is wrong, unclear or out of date, open an issue that quotes the passage and says what is wrong.
- **List your tool.** Built something around grepai? Open an issue to add it to [Community Tools](https://yoanbernabeu.github.io/grepai/community-tools/).
- **Talk about it.** A star, a post or a talk helps other people find grepai.

## Building from source

You need Go (the version in the `go` directive of `go.mod`), Git, and Docker for `make lint`.

```bash
git clone https://github.com/yoanbernabeu/grepai.git
cd grepai
make build        # builds bin/grepai
make test         # tests with the race detector
```

To try a branch that contains a fix:

```bash
git fetch origin <branch>
git checkout <branch>
make build
./bin/grepai version
```

The default build has no AST chunking or tree-sitter extraction. To include them, build with the `treesitter` tag (needs CGO and a C compiler):

```bash
go build -tags treesitter -o bin/grepai ./cmd/grepai
```

## Code of conduct

Everyone taking part in issues and discussions follows the [Code of Conduct](CODE_OF_CONDUCT.md).
