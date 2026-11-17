//go:build !darwin

package doctor

import (
	"context"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

// Away from a Mac checkSigning reports through checkSigningElsewhere only.
func checkIdentities(context.Context, *Report, config.MacOS)                     {}
func checkCertificate(*Report, config.MacOS, *project.Project)                   {}
func checkNotarization(context.Context, *Report, config.MacOS, *project.Project) {}
