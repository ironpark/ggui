package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/ironpark/ggui/cmd/ggui/internal/release"
)

// Publish is a release to make and the files to upload to it.
type Publish struct {
	Repo      string
	Tag       string
	Commitish string // the commit the tag is made at when GitHub makes it
	Name      string
	Body      string
	// Draft leaves the release a draft; otherwise it is published once
	// every file is uploaded.
	Draft      bool
	Prerelease bool
	// ReplaceAssets allows uploading to a release already published,
	// replacing its files of the same names.
	ReplaceAssets bool
	Assets        []release.Asset
	// Progress, when set, is told of each step.
	Progress func(format string, args ...any)
}

func (p *Publish) say(format string, args ...any) {
	if p.Progress != nil {
		p.Progress(format, args...)
	}
}

// Run makes the release: it creates it as a draft, or takes the draft a
// run that failed left, uploads every file, replacing one of the same
// name, and only then publishes it, so that a failure leaves a draft no
// one sees rather than a release with some of its files.
func (c *Client) Run(ctx context.Context, p Publish) (*Release, error) {
	rel, err := c.FindRelease(ctx, p.Repo, p.Tag)
	if err != nil {
		return nil, err
	}
	switch {
	case rel == nil:
		p.say("Creating the draft release %s", p.Tag)
		rel, err = c.CreateRelease(ctx, p.Repo, NewRelease{
			TagName: p.Tag, TargetCommitish: p.Commitish, Name: p.Name, Body: p.Body,
			Draft: true, Prerelease: p.Prerelease,
		})
		if err != nil {
			return nil, err
		}
	case rel.Draft:
		p.say("Reusing the draft release %s", p.Tag)
		patch := map[string]any{"name": p.Name, "body": p.Body, "prerelease": p.Prerelease}
		if p.Commitish != "" {
			patch["target_commitish"] = p.Commitish
		}
		assets := rel.Assets
		if rel, err = c.UpdateRelease(ctx, p.Repo, rel.ID, patch); err != nil {
			return nil, err
		}
		if rel.Assets == nil {
			rel.Assets = assets
		}
	case !p.ReplaceAssets:
		return nil, fmt.Errorf("the release %s of %s is published already (%s): bump app.version, or give --replace-assets to replace its files", p.Tag, p.Repo, rel.HTMLURL)
	default:
		p.say("Replacing the files of the published release %s", p.Tag)
	}

	for _, a := range p.Assets {
		if err := c.upload(ctx, &p, rel, a); err != nil {
			return rel, err
		}
	}

	if rel.Draft && !p.Draft {
		p.say("Publishing %s", p.Tag)
		id := rel.ID
		if rel, err = c.UpdateRelease(ctx, p.Repo, id, map[string]any{"draft": false}); err != nil {
			return nil, fmt.Errorf("every file is uploaded, but publishing the draft failed; publish it on GitHub or run again: %w", err)
		}
	}
	return rel, nil
}

// upload uploads a, deleting first an asset of its name: one an earlier
// run uploaded, or one a failed upload left half made.
func (c *Client) upload(ctx context.Context, p *Publish, rel *Release, a release.Asset) error {
	existing := rel.Assets
	for attempt := 0; ; attempt++ {
		for _, e := range existing {
			if e.Name == a.Name {
				p.say("Deleting the old %s", a.Name)
				if err := c.DeleteAsset(ctx, p.Repo, e.ID); err != nil && !IsNotFound(err) {
					return err
				}
			}
		}
		p.say("Uploading %s (%s)", a.Name, release.HumanSize(a.Size))
		up, err := c.UploadAsset(ctx, rel, a.Path, a.Name)
		if err == nil {
			if a.Size > 0 && up.Size != 0 && up.Size != a.Size {
				return fmt.Errorf("uploading %s: GitHub has %d bytes of %d", a.Name, up.Size, a.Size)
			}
			return nil
		}
		// GitHub may keep a broken asset of the name after an upload
		// that failed, which refuses another of that name: delete it
		// and try once more.
		if e, ok := errors.AsType[*Error](err); !ok || e.Status != http.StatusUnprocessableEntity || attempt > 0 {
			return fmt.Errorf("uploading %s: %w", a.Name, err)
		}
		if existing, err = c.ListAssets(ctx, p.Repo, rel.ID); err != nil {
			return err
		}
	}
}

// Plan lists the requests Run would make, for a dry run, without the
// replies it would read: the release's ID is shown as {id}.
func Plan(api string, p Publish) []string {
	api = trimSlash(api)
	repo := repoPath(p.Repo)
	lines := []string{
		"GET    " + api + repo + "/releases?per_page=100  (look for a release or draft at " + p.Tag + ")",
		fmt.Sprintf("POST   %s%s/releases  (draft, tag_name %s, target_commitish %s, prerelease %t; or PATCH the draft an earlier run left)",
			api, repo, p.Tag, orNone(p.Commitish), p.Prerelease),
	}
	for _, a := range p.Assets {
		lines = append(lines, fmt.Sprintf("POST   %s%s/releases/{id}/assets?name=%s  (%s, %s)",
			UploadsURL(api), repo, a.Name, ContentType(a.Name), strconv.FormatInt(a.Size, 10)+" bytes"))
	}
	if !p.Draft {
		lines = append(lines, "PATCH  "+api+repo+"/releases/{id}  (draft false: publish)")
	}
	return lines
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
