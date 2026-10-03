# Legacy local toolkit override

This is not the current development workflow. Use
[toolkit pseudo-versions](toolkit-pseudo-versions.md): push the toolkit commit to
origin, pin it with `go get <module>@<commit>` in the consumer, and build the normal
API image. After the provider merges, adopt its CI-published release tag before
merging the consumer.

Do not start new work with `toolkit-local-override.sh`, `local-toolkit/`,
`RPG_TOOLKIT_PATH`/`RPG_TOOLKIT_REF`, `go.work` or `Dockerfile.local-toolkit`.
The legacy helper remains only for handling an existing owned override; this
page does not authorize deleting existing environments or other developers'
sources. The old instructions remain available in Git history.
