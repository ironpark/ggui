cask "hello-world" do
  arch arm: "arm64", intel: "amd64"

  version "1.2.0"
  sha256 arm:   "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
         intel: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

  url "https://github.com/alice/hello/releases/download/v#{version}/hello-world-#{version}-macos-#{arch}.zip",
      verified: "github.com/alice/hello/"
  name "Hello World"
  desc "Friendly greeter"
  homepage "https://hello.example.com"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: ">= :sonoma"

  app "Hello World.app"

  zap trash: [
    "~/Library/Application Support/com.example.hello",
    "~/Library/Preferences/com.example.hello.plist",
    "~/Library/Saved Application State/com.example.hello.savedState",
  ]
end
