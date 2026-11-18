#!/bin/sh
# formula.sh VERSION writes the Homebrew formula of the ggui command at
# VERSION (vX.Y.Z) to standard output.
#
# The formula builds the command from source, as Homebrew means formulae
# to; a release's prebuilt binaries would be a cask's. It fetches the
# command's module zip from the Go module proxy rather than the whole
# repository: the command is a module of its own, about 160 KB, where
# the repository is 20 MB of fonts, icons and examples; the build then
# downloads its dependencies, zapp's module zip, about 40 MB, the most
# of them. Go is a dependency at run time too, since the command builds
# applications.
set -eu
version=$1
url="https://proxy.golang.org/github.com/ironpark/ggui/cmd/ggui/@v/${version}.zip"
sum=$(curl -fsSL "$url" | shasum -a 256 | cut -d' ' -f1)
cat <<EOF
# typed: false
# frozen_string_literal: true

# Written by the release workflow of github.com/ironpark/ggui; edits made
# here are overwritten by the next release.
class Ggui < Formula
  desc "Create, run, check and bundle ggui applications"
  homepage "https://github.com/ironpark/ggui"
  url "${url}"
  version "${version#v}"
  sha256 "${sum}"
  license "Apache-2.0"

  depends_on "go"

  def install
    # The module zip holds the module below its path and version.
    cd "ironpark/ggui/cmd/ggui@v#{version}" do
      ENV["GOWORK"] = "off"
      # Built as the release binaries are; the command needs no cgo.
      ENV["CGO_ENABLED"] = "0"
      system "go", "build", *std_go_args(ldflags: "-s -w -X main.releaseVersion=v#{version}")
    end
  end

  test do
    assert_match "v#{version}", shell_output("#{bin}/ggui version")
    system bin/"ggui", "help"
  end
end
EOF
