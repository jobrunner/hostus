package httpx

import (
	"io"
	"net/http"
)

// uiPlaceholder is the document served at "/" while the console itself is
// still being built (SP8 Task 2 replaces it with the embedded assets). It
// exists so the toggle has something real to switch on and off.
const uiPlaceholder = `<!doctype html>
<html lang="de">
<meta charset="utf-8">
<title>hostus Testkonsole</title>
<h1>hostus Testkonsole</h1>
<p>Die Konsole wird in SP8 Task 2 eingebettet.</p>
</html>
`

// handleUI serves the embedded test console. It is a plain HTTP adapter:
// it never reaches into the application or domain layer, because the
// console talks to the same public /v1 API a browser would.
func handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, uiPlaceholder)
}
