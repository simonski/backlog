class Backlog < Formula
  desc "Live dashboard for agentic harness progress"
  homepage "https://github.com/simonski/backlog"
  url "https://github.com/simonski/backlog.git",
      tag:      "v0.3.2",
      revision: "a58c62836bed45f927c6f558ff20bc73291c8a73",
      using:    :git

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args, "."
  end

  test do
    assert_match "\"server_version\":\"#{version}\"", shell_output("#{bin}/backlog version")
  end
end
