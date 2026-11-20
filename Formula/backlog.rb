class Backlog < Formula
  desc "Live dashboard for agentic harness progress"
  homepage "https://github.com/simonski/backlog"
  url "https://github.com/simonski/backlog.git",
      tag:      "v0.3.3",
      revision: "150866622bf35660abf945f679f0a4fb624315e1",
      using:    :git

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args, "."
  end

  test do
    assert_match "\"server_version\":\"#{version}\"", shell_output("#{bin}/backlog version")
  end
end
