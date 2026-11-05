package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/marcuoli/go-versioning/buildcounter"
	"github.com/marcuoli/go-versioning/gitversion"
)

func counterSeed(ctx context.Context, repo *gitversion.Repository, file string, commits bool, branch, evidence string) (*buildcounter.Seed, error) {
	if file != "" && commits {
		return nil, errors.New("--seed-file and --seed-commits are mutually exclusive")
	}
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if !stat.Mode().IsRegular() || stat.Size() > 16*1024 {
			return nil, errors.New("seed file must be a bounded regular JSON file")
		}
		data, err := io.ReadAll(io.LimitReader(f, 16*1024+1))
		if err != nil {
			return nil, err
		}
		if len(data) > 16*1024 {
			return nil, errors.New("seed file exceeds supported size")
		}
		// Seed is a flat object. Reject duplicate evidence fields instead of silently
		// replacing an earlier verified baseline with the decoder's final value.
		keys := json.NewDecoder(bytes.NewReader(data))
		token, err := keys.Token()
		if err != nil || token != json.Delim('{') {
			return nil, errors.New("seed file must be a JSON object")
		}
		seen := map[string]bool{}
		for keys.More() {
			key, e := keys.Token()
			if e != nil {
				return nil, e
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return nil, errors.New("seed file has duplicate fields")
			}
			seen[name] = true
			var value json.RawMessage
			if e = keys.Decode(&value); e != nil {
				return nil, e
			}
		}
		var seed buildcounter.Seed
		d := json.NewDecoder(bytes.NewReader(data))
		d.DisallowUnknownFields()
		if err = d.Decode(&seed); err != nil {
			return nil, err
		}
		var extra any
		if err = d.Decode(&extra); err != io.EOF {
			return nil, errors.New("seed file has trailing data")
		}
		if seed.Method != "legacy" {
			return nil, errors.New("seed files require legacy method; use --seed-commits for verified Git fallback")
		}
		return &seed, nil
	}
	if commits {
		if evidence == "" {
			return nil, errors.New("--seed-commits requires --evidence describing prior investigation and absence of a legacy method/value")
		}
		e, err := repo.CommitCount(ctx, branch)
		if err != nil {
			return nil, err
		}
		return &buildcounter.Seed{Number: e.Count, Method: "git-commits", Commit: e.Commit, Command: e.Command, Evidence: evidence, CheckedTime: e.CheckedTime}, nil
	}
	if evidence != "" {
		return nil, errors.New("--evidence requires --seed-commits; legacy evidence belongs in --seed-file")
	}
	return nil, nil
}
