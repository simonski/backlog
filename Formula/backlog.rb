require "json"

class Backlog < Formula
  desc "Live dashboard for agentic harness progress"
  homepage "https://github.com/simonski/backlog"
  url "https://github.com/simonski/backlog.git",
      tag:      "v0.4.0",
      revision: "c383ff8d8ce734551f6f76cbd8e14d76cd8c4ada",
      using:    :git
  head "https://github.com/simonski/backlog.git", branch: "main", using: :git

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(output: bin/"backlog"), "."
  end

  test do
    if build.head?
      versions = JSON.parse(shell_output("#{bin}/backlog version -json"))
      assert_match(/\A\d+\.\d+\.\d+\z/, versions.fetch("client_version"))
      assert_match(/\A\d+\.\d+\.\d+\z/, versions.fetch("server_version"))
      assert_match "-start", shell_output("#{bin}/backlog setup -help 2>&1")
      assert_match "init", shell_output("#{bin}/backlog project")
      assert_match "menu", shell_output("#{bin}/backlog -help") if OS.mac?
    else
      versions = JSON.parse(shell_output("#{bin}/backlog version"))
      assert_equal version.to_s, versions.fetch("server_version")
    end
  end
end
