# GitHub copy

The Gitea repository is the source of truth. The public copy is
[`github.com/marcuoli/go-versioning`](https://github.com/marcuoli/go-versioning).
Make changes in Gitea and let `.gitea/workflows/mirror-github.yml` copy branches
and tags to GitHub. Treat the GitHub repository as a distribution copy.

## Existing-mirror configuration

This repository's public copy is already bootstrapped. The current workflow
requires the exact historical `v0.1.0` identities documented below, including
the destination's existing lightweight tag. An empty replacement repository
will fail before any ref update. Restore approved historical refs through a
deliberate recovery procedure before enabling this workflow on a recreated
copy; do not remove the guard to bypass missing history.

1. Use the existing public GitHub repository with its recorded immutable
   historical refs. Publish only material approved for public distribution.
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
all branches and tags into a fresh directory and explicitly checks out `main`.
This avoids replacing an annotated tag ref with a tag event's peeled commit SHA.
Source authentication uses the Gitea job context; the GitHub deploy key is
available only to the copy step.

If the runner does not already trust the source HTTPS certificate issuer, add the
repository Actions secret `SOURCE_CA_PEM` with the required PEM CA certificates
from an independently verified trust chain. Both Gitea workflows optionally
combine those certificates with the runner's existing system CA bundle before
checkout. Git and Node.js use the temporary mode-600 bundle, which an always-run
cleanup step removes afterward. Certificate and hostname verification stay
enabled. Without the secret, the workflows keep their normal system trust
settings. Keep source certificate details and infrastructure addresses out of
tracked files. Hard runner termination can prevent cleanup, so use disposable or
appropriately cleaned runner storage.

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

The already-published `v0.1.0` has one frozen historical exception. Its source
annotated tag object is `71d1b1e9fd62f5f25e425746370a8af074b0e122`, targeting
commit `71b1ece94d0b85eec80cb794bdb72d14272bc951`; GitHub's existing lightweight
`v0.1.0` ref points directly to that same commit. The initial tag-event checkout
lost the annotation before copying. Both published refs remain immutable.

The workflow logs retention and omits only that tag from its push when the local
source tag object, local peeled commit, and GitHub direct ref exactly match
those identities. This permits later branch and tag copies without replacing
`v0.1.0`. A different or missing source tag, source commit, or destination ref
fails closed before any branch or tag is pushed. All other tag conflicts still
reject the atomic push, even when two different tag objects refer to the same
commit. Future annotated tags retain their exact source tag object IDs on GitHub.

The SSH key is written to a temporary mode-600 file and removed by an exit trap.
The SSH connection uses a pinned GitHub Ed25519 host key and strict host checking;
it does not trust keys discovered at runtime. If GitHub rotates that host key,
verify the replacement against GitHub's published keys before changing the
workflow. Hard runner termination can prevent exit traps, so use disposable or
appropriately cleaned runner storage.

A green local test does not prove copying works. Acceptance requires a successful
Gitea Actions run with the configured secret and matching branch/tag object IDs
on GitHub, apart from the frozen historical `v0.1.0` exception above.
The workflow cannot execute until its repository secret and runner are available.

## References

- [Gitea Actions secrets](https://docs.gitea.com/usage/actions/secrets/)
- [Gitea Actions concurrency support](https://blog.gitea.com/release-of-1.26.0/)
- [GitHub deploy keys](https://docs.github.com/en/authentication/connecting-to-github-with-ssh/managing-deploy-keys)
- [GitHub SSH host keys](https://docs.github.com/en/authentication/keeping-your-account-and-data-secure/githubs-ssh-key-fingerprints)
- [Pinned checkout action](https://github.com/actions/checkout/tree/11bd71901bbe5b1630ceea73d27597364c9af683)
