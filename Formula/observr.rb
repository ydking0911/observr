# Homebrew formula for observrd — the observr collector binary.
#
# To use this formula before it's in the official Homebrew tap:
#   brew tap ydking0911/observr https://github.com/ydking0911/observr
#   brew install ydking0911/observr/observr
#
# Once a Homebrew tap repo exists at github.com/ydking0911/homebrew-tap,
# update the sha256 values after each release and copy this file there.

class Observr < Formula
  desc "Zero-config local observability collector for AI agents and developers"
  homepage "https://github.com/ydking0911/observr"
  version "0.2.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-darwin-arm64"
      sha256 "REPLACE_WITH_SHA256_AFTER_RELEASE"
    end
    on_intel do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-darwin-amd64"
      sha256 "REPLACE_WITH_SHA256_AFTER_RELEASE"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-linux-arm64"
      sha256 "REPLACE_WITH_SHA256_AFTER_RELEASE"
    end
    on_intel do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-linux-amd64"
      sha256 "REPLACE_WITH_SHA256_AFTER_RELEASE"
    end
  end

  def install
    bin.install stable.url.split("/").last => "observrd"
  end

  test do
    assert_match "observrd", shell_output("#{bin}/observrd --help 2>&1", 2)
  end
end
