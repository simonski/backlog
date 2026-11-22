require "json"

class Backlog < Formula
  desc "Live dashboard for agentic harness progress"
  homepage "https://github.com/simonski/backlog"
  url "https://github.com/simonski/backlog.git",
      tag:      "v0.16.0",
      revision: "e35a7cabd72cc50a2819a14d9f0d5444ae1c4456",
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
      assert_equal "0.16.0", versions.fetch("client_version")
      assert_equal "0.16.0", versions.fetch("server_version")
    end
    assert_match(/\A\d+\.\d+\.\d+\z/, versions.fetch("protocol_semver"))
    assert_match "-start", shell_output("#{bin}/backlog setup -help 2>&1")
    assert_match(/^\s+-d(?:\s|$)/, shell_output("#{bin}/backlog serve -help 2>&1"))
    assert_match "Usage: backlog stop", shell_output("#{bin}/backlog stop -help 2>&1")
    assert_match "Usage: backlog initdb", shell_output("#{bin}/backlog initdb -help 2>&1")
    assert_match "init", shell_output("#{bin}/backlog project")
    assert_match "Usage: backlog upgrade", shell_output("#{bin}/backlog upgrade -help 2>&1")
    assert_match "menu", shell_output("#{bin}/backlog -help") if OS.mac?
  end
end
