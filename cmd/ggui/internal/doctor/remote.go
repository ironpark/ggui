package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
)

// checkRemote says whether ggui dev can show the project's app in its
// viewer, the window that stays while the app restarts, and why not.
func checkRemote(ctx context.Context, r *Report, o Options, p projectInfo) {
	const name = "remote display"
	if err := viewer.Supported(); err != nil {
		r.ok(name, err.Error()+"; ggui dev opens the app's own window")
		return
	}
	if p.ggui == nil {
		return // no project, or one without ggui: said above
	}
	if proj, err := project.Open(o.Dir, "", o.Config); err == nil && proj.Config.Dev.Display == "window" {
		r.ok(name, `off: dev.display is "window"`)
		return
	}
	cache, err := viewer.CacheDir()
	if err != nil {
		r.warn(name, "no cache directory for the viewer: "+err.Error(), "")
		return
	}
	b, err := viewer.Resolve(ctx, o.Dir, cache)
	var u *viewer.Unavailable
	switch {
	case errors.As(err, &u):
		r.warn(name, u.Reason+"; ggui dev opens the app's own window",
			"for the viewer, which keeps the window while the app restarts: go get "+gguiPath+"@latest")
	case err != nil:
		r.warn(name, err.Error(), "")
	default:
		where := "built by the first ggui dev, into " + filepath.Dir(filepath.Dir(b.Path))
		if _, err := os.Stat(b.Path); err == nil {
			where = "built, in " + filepath.Dir(b.Path)
		}
		r.ok(name, fmt.Sprintf("ggui dev shows the app in its viewer (%s)", where))
	}
}
