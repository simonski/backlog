class Backlog < Formula
  desc "Live dashboard for agentic harness progress"
  homepage "https://github.com/simonski/backlog"
  url "https://github.com/simonski/backlog.git",
      tag:      "v0.4.0",
      revision: "c383ff8d8ce734551f6f76cbd8e14d76cd8c4ada",
      using:    :git

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args, "."
  end

  test do
    assert_match "\"server_version\":\"#{version}\"", shell_output("#{bin}/backlog version")
  end
end
