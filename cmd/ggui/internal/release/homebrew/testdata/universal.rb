cask "hello-world" do
  version "1.2.0"
  sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

  url "https://github.com/alice/hello/releases/download/v#{version}/hello-world-#{version}-macos-universal.dmg"
  name "Hello World"
  desc "Friendly greeter"
  homepage "https://github.com/alice/hello"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: ">= :ventura"

  app "Hello World.app"

  zap trash: [
    "~/Library/Application Support/com.example.hello",
    "~/Library/Preferences/com.example.hello.plist",
    "~/Library/Saved Application State/com.example.hello.savedState",
  ]
end
