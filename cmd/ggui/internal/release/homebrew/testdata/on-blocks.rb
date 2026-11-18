cask "hello-world" do
  version "1.2.0"

  on_arm do
    sha256 "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

    url "https://github.com/alice/hello/releases/download/v#{version}/hello-world-#{version}-macos-arm64.dmg",
        verified: "github.com/alice/hello/"
  end
  on_intel do
    sha256 "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

    url "https://github.com/alice/hello/releases/download/v#{version}/hello-world-#{version}-macos-amd64.zip",
        verified: "github.com/alice/hello/"
  end

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
