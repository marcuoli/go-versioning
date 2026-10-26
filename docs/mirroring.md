# GitHub copy

The Gitea repository is the source of truth. The public copy is
[`github.com/marcuoli/go-versioning`](https://github.com/marcuoli/go-versioning).
Make changes in Gitea and let `.gitea/workflows/mirror-github.yml` copy branches
and tags to GitHub. Treat the GitHub repository as a distribution copy.

## Setup

1. Create the empty public GitHub repository without an initial README, license,
   or `.gitignore` commit. Publish only material approved for public distribution.
2. Generate a dedicated SSH key pair for this repository. Add its public key as a
   GitHub deploy key with **Allow write access** enabled. Do not reuse an account
   SSH key or give Gitea a broad GitHub access token.
3. Enable Gitea Actions for the source repository. Save the complete private key
   as the repository Actions secret `GH_MIRROR_SSH_KEY`. Keep the key out of
   commits, command arguments, logs, artifacts, and documentation.
4. Provide an enabled Linux runner with the `ubuntu-latest` label, Bash, Git,
   OpenSSH, and Node.js 20 for the pinned checkout action. The runner must reach
   the source repository, download the action from GitHub over HTTPS, and reach
   `github.com` over SSH on port 22. Use Gitea 1.26 or newer for concurrency.
5. Push a branch or tag, or use **Run workflow** on the source repository's
   Actions page. Verify that the run succeeds and compare source and destination
   branch and tag object IDs with `git ls-remote`.

The checkout action is pinned to the verified `actions/checkout` v4.2.2 commit
`11bd71901bbe5b1630ceea73d27597364c9af683`. It checks out complete history and
all branches and tags into a fresh directory. Source authentication uses the
Gitea job context; the GitHub deploy key is available only to the copy step.

## Copy behavior

Every branch or tag push requests a copy. One concurrency group serializes all
copy runs without cancelling a running copy. A queued run copies the source refs
observed when its checkout starts, so this is eventual synchronization rather
than a record of every intermediate push. A later push or manual run retries a
missed or failed copy.

The workflow explicitly maps source branch refs to GitHub branches and source
tag refs to GitHub tags. It excludes the symbolic `origin/HEAD` alias and other
symbolic remote refs. It does not copy pull request refs, other internal Git
refs, issues, pull requests, release pages, release assets, repository settings,
submodule repositories, or Git LFS objects. It never uses `git push --mirror`.

All ref updates are one ordinary atomic Git push. Branches must fast-forward;
a non-fast-forward branch conflict or a different existing tag fails the whole
push. The workflow never forces a tag replacement and never deletes remote
branches or tags. A deleted source ref therefore remains on GitHub until an
operator deliberately reconciles it. GitHub protection rules can also reject a
copy. Resolve conflicts explicitly after checking the source of truth, then run
the workflow again. Do not automatically force or delete refs to clear a failure.

The SSH key is written to a temporary mode-600 file and removed by an exit trap.
The SSH connection uses a pinned GitHub Ed25519 host key and strict host checking;
it does not trust keys discovered at runtime. If GitHub rotates that host key,
verify the replacement against GitHub's published keys before changing the
workflow. Hard runner termination can prevent exit traps, so use disposable or
appropriately cleaned runner storage.

A green local test does not prove copying works. Acceptance requires a successful
Gitea Actions run with the configured secret and matching branch/tag object IDs
on GitHub. The workflow cannot execute until its repository secret and runner
are available.

## References

- [Gitea Actions secrets](https://docs.gitea.com/usage/actions/secrets/)
- [Gitea Actions concurrency support](https://blog.gitea.com/release-of-1.26.0/)
- [GitHub deploy keys](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/managing-deploy-keys)
- [GitHub SSH host keys](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/githubs-ssh-key-fingerprints)
- [Pinned checkout action](https://github.com/actions/checkout/tree/11bd71901bbe5b1630ceea73d27597364c9af683)
