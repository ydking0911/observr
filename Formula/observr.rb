# Homebrew formula for observrd — the observr collector binary.
#
# To use this formula before it's in the official Homebrew tap:
#   brew tap ydking0911/observr https://github.com/ydking0911/observr
#   brew install ydking0911/observr/observr
#
# Once a Homebrew tap repo exists at github.com/ydking0911/homebrew-tap,
# update the sha256 values after each release and copy this file there.

class Observr < Formula
  desc "Audit trail and causal attribution for AI agents"
  homepage "https://github.com/ydking0911/observr"
  version "0.5.0"
  license "MIT"

  on_macos do
    on_arm do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-darwin-arm64"
      sha256 "5501aa7f599007b2cd6954b8bab0d77b0dddb7774577a011a0ec1352e5bbea5f"
    end
    on_intel do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-darwin-amd64"
      sha256 "3e354c9f60a6cae0cbd52cec73709d8f2b3a342c2d055fee8f8322e6330b4ee7"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-linux-arm64"
      sha256 "a960d45bbfa67744da775af0204f3ae897fcbf77bc691c12e56dd9f2405c30a1"
    end
    on_intel do
      url "https://github.com/ydking0911/observr/releases/download/v#{version}/observrd-linux-amd64"
      sha256 "62ef6edcd7a0078855e9de3bdaa9fc097bda70b135e101a6a0272f6c6315939f"
    end
  end

  def install
    bin.install stable.url.split("/").last => "observrd"
  end

  test do
    assert_match "observrd", shell_output("#{bin}/observrd --help 2>&1", 2)
  end
end
