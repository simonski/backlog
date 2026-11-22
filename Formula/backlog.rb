require "json"

class Backlog < Formula
  desc "Live dashboard for agentic harness progress"
  homepage "https://github.com/simonski/backlog"
  url "https://github.com/simonski/backlog.git",
      tag:      "v0.14.0",
      revision: "48d8c1424767e41832479e5d121b024f8a308ba2",
      using:    :git
  head "https://github.com/simonski/backlog.git", branch: "main", using: :git

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(output: bin/"backlog"), "."
  end

  test do
    versions = JSON.parse(shell_output("#{bin}/backlog version -json"))
    if build.head?
      assert_match(/\A\d+\.\d+\.\d+\z/, versions.fetch("client_version"))
      assert_match(/\A\d+\.\d+\.\d+\z/, versions.fetch("server_version"))
    else
      assert_equal version.to_s, versions.fetch("client_version")
      assert_equal version.to_s, versions.fetch("server_version")
    end
    assert_match "-start", shell_output("#{bin}/backlog setup -help 2>&1")
    assert_match "init", shell_output("#{bin}/backlog project")
    assert_match "Usage: backlog upgrade", shell_output("#{bin}/backlog upgrade -help 2>&1")
    assert_match "menu", shell_output("#{bin}/backlog -help") if OS.mac?
  end
end
