package main

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/marcuoli/go-versioning/buildcounter"
	"github.com/marcuoli/go-versioning/gitversion"
)

func counterMain(ctx context.Context, source *gitversion.Repository, explicit, branch string) (string, error) {
	selected := source
	if explicit != "" {
		var e error
		selected, e = gitversion.NewContext(ctx, explicit)
		if e != nil {
			return "", e
		}
	}
	root, e := selected.PrimaryMain(ctx, branch)
	if e != nil {
		return "", e
	}
	if explicit != "" {
		given, e := filepath.Abs(explicit)
		if e != nil {
			return "", e
		}
		given, e = filepath.EvalSymlinks(given)
		if e != nil {
			return "", e
		}
		if given != root {
			return "", errors.New("--main-repo must designate primary permanent main, not a linked checkout")
		}
	}
	primary, e := gitversion.NewContext(ctx, root)
	if e != nil {
		return "", e
	}
	if e = primary.CheckCounterFiles(ctx); e != nil {
		return "", e
	}
	return root, nil
}

func checkCounterMain(ctx context.Context, s *buildcounter.Store, source *gitversion.Repository, explicit, branch string) error {
	root, e := counterMain(ctx, source, explicit, branch)
	if e != nil {
		return e
	}
	_, e = s.CheckLocation(ctx, filepath.Join(root, "VERSION"))
	return e
}
