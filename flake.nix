{
  description = "hostus - Go project with reproducible development environment";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        # Go Version (aktuell stabil)
        go = pkgs.go_1_26;

        # Entwicklungswerkzeuge
        devTools = with pkgs; [
          # Go Toolchain
          go
          gopls                    # Language Server
          gotools                  # goimports, godoc, etc.
          go-tools                 # staticcheck
          delve                    # Debugger

          # Linting & Analyse
          golangci-lint            # Meta-Linter
          govulncheck              # Vulnerability Scanner

          # Testing
          gotestsum                # Bessere Test-Ausgabe
          go-junit-report          # JUnit Reports

          # Build & Release
          goreleaser               # Release Automation

          # CI/CD
          act                      # GitHub Actions lokal ausführen
          actionlint               # GitHub Actions Linter

          # Utilities
          jq                       # JSON Verarbeitung
          sqlite                   # SQLite CLI (für Debugging)
        ];

      in
      {
        # Development Shell
        devShells.default = pkgs.mkShell {
          buildInputs = devTools;

          shellHook = ''
            # Go Umgebung
            # WICHTIG: Caches liegen AUSSERHALB des Arbeitsbaums (nicht $PWD/.go).
            # Grund: Tools wie gremlins (mutation testing) kopieren das gesamte
            # Modulverzeichnis inkl. .go/ in ein temporäres Arbeitsverzeichnis, um
            # Mutationen anzuwenden. Die im Go-Modul-Cache enthaltenen Dateien sind
            # read-only (Modus 0444); eine Kopie davon innerhalb des Repos brach
            # diesen Kopiervorgang. Ablage unter XDG_CACHE_HOME entspricht zudem
            # dem üblichen Go-Setup.
            export GOPATH="''${XDG_CACHE_HOME:-$HOME/.cache}/hostus/go"
            export GOBIN="$GOPATH/bin"
            export PATH="$GOBIN:$PATH"

            # Cache Verzeichnisse
            export GOCACHE="$GOPATH/cache"
            export GOMODCACHE="$GOPATH/mod"

            # Erstelle Verzeichnisse falls nicht vorhanden
            mkdir -p "$GOPATH" "$GOBIN" "$GOCACHE" "$GOMODCACHE"

            echo ""
            echo "🔧 hostus Entwicklungsumgebung"
            echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
            echo "Go Version:      $(go version | cut -d' ' -f3)"
            echo "golangci-lint:   $(golangci-lint --version | head -1)"
            echo ""

            # Dynamisch Make-Targets aus Makefile extrahieren und anzeigen
            if [ -f Makefile ]; then
              echo "Verfügbare Make-Targets:"
              echo ""
              grep -E '^[a-zA-Z_-]+:.*?## .*$$' Makefile | \
                awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $1, $2}' | \
                head -15
              TOTAL=$(grep -cE '^[a-zA-Z_-]+:.*?## .*$$' Makefile 2>/dev/null || echo "0")
              if [ "$TOTAL" -gt 15 ]; then
                echo ""
                echo "  ... und $((TOTAL - 15)) weitere (siehe: make help)"
              fi
              echo ""
            fi
          '';

          # Pure-Go via modernc.org/sqlite - keine CGO-Abhängigkeit, distroless-fähig
          CGO_ENABLED = "0";
        };

        # Packages
        packages.default = pkgs.buildGoModule {
          pname = "hostus";
          version = "0.2.0";
          src = ./.;

          # Wird beim ersten Build aktualisiert
          vendorHash = null;

          CGO_ENABLED = 0;

          meta = with pkgs.lib; {
            description = "Hostus - Taxonomy gateway service";
            homepage = "https://github.com/jobrunner/hostus";
            license = licenses.mit;
            maintainers = [ ];
          };
        };
      }
    );
}
