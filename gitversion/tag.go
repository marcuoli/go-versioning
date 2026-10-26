package gitversion

import (
	"context"
	"errors"
	"regexp"
	"strings"

	versioning "github.com/marcuoli/go-versioning"
)

// TagResult records verified local and remote presence. Created is true only
// when this call created the local tag; it stays true if its subsequent push fails.
type TagResult struct {
	Plan    Plan `json:"plan"`
	Local   bool `json:"local"`
	Remote  bool `json:"remote"`
	Created bool `json:"created"`
}

var remoteName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func (r *Repository) validateRemote(ctx context.Context, remote string) error {
	if !remoteName.MatchString(remote) || strings.Contains(remote, "..") {
		return errors.New("remote must be a configured name")
	}
	names, err := r.run(ctx, "remote inventory", "remote")
	if err != nil {
		return err
	}
	for _, name := range strings.Split(names, "\n") {
		if name == remote {
			fetch, err := r.run(ctx, "remote fetch destination", "remote", "get-url", "--all", remote)
			if err != nil {
				return err
			}
			push, err := r.run(ctx, "remote push destination", "remote", "get-url", "--push", "--all", remote)
			if err != nil {
				return err
			}
			if fetch == "" || push != fetch || strings.ContainsAny(fetch, "\r\n") {
				return errors.New("remote requires one matching fetch and push destination")
			}
			return nil
		}
	}
	return errors.New("remote name is not configured")
}
func (r *Repository) remoteTags(ctx context.Context, remote string) ([]applicationTag, error) {
	value, err := r.run(ctx, "remote tag inventory", "ls-remote", "--tags", remote)
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	peeled := map[string]string{}
	for _, line := range strings.Split(value, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !fullCommit.MatchString(fields[0]) || !strings.HasPrefix(fields[1], "refs/tags/") {
			return nil, errors.New("invalid remote tag inventory")
		}
		name := strings.TrimPrefix(fields[1], "refs/tags/")
		if strings.HasSuffix(name, "^{}") {
			peeled[strings.TrimSuffix(name, "^{}")] = fields[0]
		} else {
			refs[name] = fields[0]
		}
	}
	var result []applicationTag
	for name, identity := range refs {
		if !candidate(name) {
			continue
		}
		v, legacy, err := versioning.ParseTag(name)
		if err != nil {
			return nil, errors.New("malformed remote application tag")
		}
		if p, ok := peeled[name]; ok {
			identity = p
		}
		result = append(result, applicationTag{name: name, commit: identity, version: v, legacy: legacy})
	}
	return result, nil
}
func validateRemoteTags(plan Plan, local, remote []applicationTag) (bool, error) {
	locals := map[string]string{}
	for _, tag := range local {
		locals[tag.name] = tag.commit
	}
	predecessorPresent := plan.PreviousTag == ""
	expected := false
	targetCount := 0
	for _, tag := range remote {
		if tag.commit == plan.Commit {
			targetCount++
			if tag.name != plan.Tag {
				return false, errors.New("remote target has a different application tag")
			}
		}
		if tag.name == plan.Tag {
			if tag.commit != plan.Commit {
				return false, errors.New("remote expected tag belongs to another commit")
			}
			expected = true
		}
		if localCommit, ok := locals[tag.name]; !ok || localCommit != tag.commit {
			return false, errors.New("remote application tags must exist locally with matching commits")
		}
		if tag.name == plan.PreviousTag && tag.commit == locals[plan.PreviousTag] {
			predecessorPresent = true
		}
	}
	if targetCount > 1 {
		return false, errors.New("remote target has ambiguous application tags")
	}
	if !predecessorPresent {
		return false, errors.New("predecessor application tag is absent from remote")
	}
	return expected, nil
}

// Tag validates the plan, creates a lightweight immutable local tag if needed,
// and optionally pushes only that exact ref to a configured named remote.
// Remote application history is checked before any local tag creation. A failed
// push leaves the local tag intact and returns its state alongside the error.
func (r *Repository) Tag(ctx context.Context, o PlanOptions, remote string) (TagResult, error) {
	plan, err := r.Next(ctx, o)
	if err != nil {
		return TagResult{}, err
	}
	result := TagResult{Plan: plan}
	local, err := r.tags(ctx)
	if err != nil {
		return result, err
	}
	for _, tag := range local {
		if tag.name == plan.Tag && tag.commit == plan.Commit {
			result.Local = true
		}
	}
	if remote != "" {
		if err = r.validateRemote(ctx, remote); err != nil {
			return result, err
		}
		var tags []applicationTag
		tags, err = r.remoteTags(ctx, remote)
		if err != nil {
			return result, err
		}
		result.Remote, err = validateRemoteTags(plan, local, tags)
		if err != nil {
			return result, err
		}
	}
	if !result.Local {
		_, err = r.run(ctx, "local tag creation", "-c", "tag.gpgSign=false", "tag", "--", plan.Tag, plan.Commit)
		if err != nil {
			// A concurrent creator is accepted only after the entire local plan is revalidated.
			if _, checkErr := r.Next(ctx, o); checkErr != nil {
				return result, err
			}
			existing, checkErr := r.tags(ctx)
			if checkErr != nil {
				return result, err
			}
			for _, tag := range existing {
				if tag.name == plan.Tag && tag.commit == plan.Commit {
					result.Local = true
				}
			}
			if !result.Local {
				return result, err
			}
		} else {
			result.Local = true
			result.Created = true
		}
	}
	if remote == "" || result.Remote {
		return result, nil
	}
	ref := "refs/tags/" + plan.Tag
	_, pushErr := r.run(ctx, "tag push", "-c", "remote."+remote+".mirror=false", "push", "--porcelain", "--no-follow-tags", "--recurse-submodules=no", remote, ref+":"+ref)
	// Verify the published ref even when transport reported failure: a server can
	// accept a push before the client loses its connection.
	remoteAfter, verifyErr := r.remoteTags(ctx, remote)
	if verifyErr == nil {
		for _, tag := range remoteAfter {
			if tag.name == plan.Tag && tag.commit == plan.Commit {
				result.Remote = true
			}
		}
	}
	if pushErr != nil {
		return result, pushErr
	}
	if verifyErr != nil {
		return result, verifyErr
	}
	if !result.Remote {
		return result, errors.New("pushed tag could not be verified remotely")
	}
	return result, nil
}
