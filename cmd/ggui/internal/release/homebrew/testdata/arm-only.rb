cask "hello-world" do
  version "1.2.0"
  sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

  url "https://github.com/alice/hello/releases/download/v#{version}/hello-world-#{version}-macos-arm64.zip"
  name "Hello World"
  homepage "https://github.com/alice/hello"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on arch: :arm64
  depends_on macos: ">= :ventura"

  app "Hello World.app"
end
