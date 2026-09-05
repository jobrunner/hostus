"use strict";

(function () {
  /* ---------- kleine Helfer ---------- */

  function byId(id) {
    return document.getElementById(id);
  }

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) { node.className = cls; }
    if (text !== undefined && text !== null) { node.textContent = String(text); }
    return node;
  }

  function cell(tag, text, cls) {
    var node = el(tag, cls, text);
    return node;
  }

  // A header entry is either a plain string, or [label, docKey] to attach a
  // hover explanation (ⓘ) — see FIELD_DOCS.
  function table(headers) {
    var t = el("table");
    var thead = el("thead");
    var tr = el("tr");
    headers.forEach(function (h) {
      var th = el("th");
      if (Array.isArray(h)) { withDoc(th, h[0], h[1]); } else { th.textContent = h; }
      tr.appendChild(th);
    });
    thead.appendChild(tr);
    t.appendChild(thead);
    t.appendChild(el("tbody"));
    return t;
  }

  // Ein waagerecht scrollbarer Container ist ohne Tastaturfokus nur mit der
  // Maus erreichbar (WCAG 2.1.1). Er wird deshalb zum Tab-Stopp — aber NUR
  // solange er tatsächlich überläuft: eine Tabelle, die auf breitem Schirm
  // vollständig passt, wäre sonst ein stummer Halt in der Tab-Reihenfolge
  // ohne jeden Zweck. Ob sie überläuft, hängt an Fenstergröße und Inhalt und
  // kann sich jederzeit ändern, deshalb entscheidet das ein ResizeObserver
  // fortlaufend statt einmal beim Erzeugen.
  function syncScrollFocus(d) {
    if (d.scrollWidth > d.clientWidth) {
      d.tabIndex = 0;
    } else {
      d.removeAttribute("tabindex");
    }
  }

  var scrollObserver = typeof ResizeObserver === "function"
    ? new ResizeObserver(function (entries) {
      entries.forEach(function (e) { syncScrollFocus(e.target); });
    })
    : null;

  function watchScroller(d) {
    // Ohne ResizeObserver lieber dauerhaft erreichbar als gar nicht: ein
    // überflüssiger Tab-Stopp ist ein Ärgernis, ein fehlender eine Barriere.
    if (scrollObserver) { scrollObserver.observe(d); } else { d.tabIndex = 0; }
  }

  function scroller(node) {
    var d = el("div", "scroll");
    d.appendChild(node);
    watchScroller(d);
    return d;
  }

  function badge(text, kind) {
    return el("span", "badge " + kind, text);
  }

  // A pair is [label, value] or [label, value, docKey] for a hover explanation.
  function dl(pairs) {
    var d = el("dl", "kv");
    pairs.forEach(function (p) {
      if (p[1] === undefined || p[1] === null || p[1] === "") { return; }
      var row = el("div");
      var dt = el("dt");
      if (p[2]) { withDoc(dt, p[0], p[2]); } else { dt.textContent = p[0]; }
      row.appendChild(dt);
      row.appendChild(cell("dd", p[1]));
      d.appendChild(row);
    });
    return d;
  }

  function num(v, digits) {
    return typeof v === "number" ? v.toFixed(digits) : "–";
  }

  /* Diakritika-tolerante Kleinschreibung fuer den Praefixvergleich. */
  function fold(s) {
    return String(s || "").toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");
  }

  function truncate(s, n) {
    s = String(s || "");
    return s.length > n ? s.slice(0, n - 1) + "\u2026" : s;
  }

  /* Herkunft eines Konzepts OHNE sec.-Raum, aus der concept_id abgeleitet:
     ein WCVP-Backbone-Konzept oder ein CDM-Konzept, dem der sec fehlt. */
  function secSource(conceptId) {
    var pfx = String(conceptId || "").split(":")[0];
    if (pfx === "wcvp") { return "WCVP"; }
    if (pfx === "cdm") { return "CDM (ohne sec.)"; }
    return "\u2013";
  }

  /* Eine Tabellenzelle fuer einen sec.-Raum: gekuerzter Titel (voller Titel als
     Tooltip). Fehlt der sec.-Raum, wird stattdessen die Herkunft benannt (WCVP
     bzw. \u201eCDM (ohne sec.)\u201c), damit klar ist, welches Konzept gemeint ist. */
  function secTd(sec, conceptId) {
    // A missing OR empty-title sec falls back to the concept origin. Translate's
    // DTOs carry sec as a non-omitempty value object, so `sec` is truthy even
    // when the concept has no sec space; only a non-empty title is a real sec.
    if (sec && sec.title) {
      var td = cell("td", truncate(sec.title, 28), "sec");
      td.title = sec.title;
      return td;
    }
    return cell("td", secSource(conceptId), "sec source");
  }

  /* ---------- Feld-Erlaeuterungen (eine Textquelle fuer Tooltip UND Legende) ---------- */

  var FIELD_DOCS = {
    // Panel 1 \u2013 Suggest
    rank: "Taxonomischer Rang des Treffers (FAMILY/GENUS/SPECIES/SUBSPECIES; sonst OTHER, dann steht die Rohschreibweise als rank_verbatim daneben).",
    accepted: "Ob das Konzept selbst der akzeptierte Name ist (ja) oder ein Synonym, das auf ein akzeptiertes Konzept aufgeloest wurde.",
    in_area: "Positiver Verbreitungsbeleg fuers Gebiet: „ja“, wenn das Konzept selbst dort verbreitet ist ODER derselbe Name bei WCVP (der Verbreitungs-Autoritaet, akzeptiert oder Synonym) dort vorkommt. Sonst „keine Angabe“ — Verbreitung ist Praesenz-Daten, ein fehlender Eintrag ist keine belegte Abwesenheit (deshalb nie „nein“). Ohne area-Parameter immer „keine Angabe“.",
    score: "Roher SQLite-FTS5-bm25()-Wert des Treffers. Niedriger = relevanter (ein Distanzmass, keine Aehnlichkeit).",
    prefix: "Ob der ANGEZEIGTE (akzeptierte) Name mit deiner Eingabe BEGINNT (links-verankert, normalisiert). \u201enein\u201c = der Treffer kam ueber einen anderen indexierten Namen: ein Synonym, eine Aggregat-Schreibweise oder einen spaeteren Token.",
    aggregate: "Das Konzept wurde ueber eine Aggregat-Schreibweise (agg./aggr./s.l.) getroffen. Da FloraVeg-Aggregate auf die Nominatart zeigen, wird die Nominatart mit diesem Badge angezeigt.",
    sec: "sec.-Referenzraum (\u201esecundum\u201c): die Flora/Checkliste, deren Umschreibung dieses Konzept meint. Unterscheidet gleichnamige CDM-Konzepte (Common Data Model, die Cybertaxonomy-/EDIT-Plattform mit den Wisskirchen-Konzeptbeziehungen) voneinander. Hat ein Konzept keinen sec.-Raum, steht dort die Herkunft: WCVP (World Checklist of Vascular Plants) als Backbone-Konzept, oder CDM (ohne sec.) bei den seltenen CDM-Konzepten ohne sec.",
    // Panel 2 \u2013 Konzept & Synonyme
    publishable: "Darf dieser Synonym-Name in einer veroeffentlichten Synonymliste des Taxons stehen? ja = nomenklatorisch unbedenklich (kein disqualifizierender Status, Rang nicht ausgeschlossen); nein = zurueckgehalten \u2014 Grund in nom_status/Begruendung.",
    nom_status: "Nomenklatorischer Status aus der Quelle (z. B. nom. illeg., not validly publ., superfl., nom. nud.). Grundlage der Publikationsrelevanz.",
    typification: "Art der Synonymie: homotypisch (gleicher nomenklatorischer Typus \u2014 objektiv) oder heterotypisch (anderer Typus \u2014 ein taxonomisches Urteil).",
    basionym: "Ob der Name das Basionym ist: der zuerst gueltig veroeffentlichte Name, auf dem spaetere Umkombinationen beruhen.",
    reason: "Nachvollziehbare Begruendung, warum ein Synonym publizierbar ist oder zurueckgehalten wurde.",
    backbone: "Herkunfts-Backbone des Konzepts und dessen Version. wcvp = World Checklist of Vascular Plants (Kew); cdm = Common Data Model (Cybertaxonomy-/EDIT-Plattform, hier die Wisskirchen-Konzeptbeziehungen).",
    // Panel 3 \u2013 Match
    match_type: "Wie der verbatim-Name aufgeloest wurde: exact, exact_author, aggregate_alias oder fuzzy \u2014 oder unresolvable, wenn keine Zuordnung moeglich war.",
    confidence: "Konfidenz der Aufloesung (0\u20131). Hoeher = sicherer.",
    requires_review: "Ob die Aufloesung eine manuelle Pruefung braucht (etwa bei fuzzy oder Mehrdeutigkeit)."
  };

  /* Haengt an ein Label-Element (th/dt) einen dezenten Hinweis mit Tooltip. */
  function withDoc(node, labelText, docKey) {
    node.appendChild(document.createTextNode(labelText));
    var doc = FIELD_DOCS[docKey];
    if (doc) {
      var info = el("span", "info", "\u24d8");
      info.title = doc;
      node.appendChild(document.createTextNode(" "));
      node.appendChild(info);
    }
    return node;
  }

  /* Reichert statische [data-doc]-Ueberschriften (index.html) mit dem Hinweis an. */
  function enhanceDocs(root) {
    var nodes = root.querySelectorAll("[data-doc]");
    Array.prototype.forEach.call(nodes, function (n) {
      if (n.querySelector(".info")) { return; } // schon angereichert
      var label = n.textContent;
      n.textContent = "";
      withDoc(n, label, n.getAttribute("data-doc"));
    });
  }

  // Welche Felder je Panel in der Legende erklaert werden.
  var PANEL_LEGENDS = {
    "panel-suggest": ["prefix", "aggregate", "sec", "rank", "accepted", "in_area", "score"],
    "panel-concept": ["sec", "backbone", "publishable", "nom_status", "typification", "basionym", "reason"],
    "panel-match": ["match_type", "confidence", "requires_review"]
  };

  /* Haengt je Panel eine aufklappbare \u201eFelder erklaert\u201c-Liste ans Section-Ende. */
  function buildLegends() {
    Object.keys(PANEL_LEGENDS).forEach(function (pid) {
      var section = byId(pid);
      if (!section || section.querySelector(".legend")) { return; }
      var det = el("details", "legend");
      det.appendChild(el("summary", null, "Felder erkl\u00e4rt"));
      var d = el("dl", "kv");
      PANEL_LEGENDS[pid].forEach(function (k) {
        if (!FIELD_DOCS[k]) { return; }
        var row = el("div");
        row.appendChild(cell("dt", k));
        row.appendChild(cell("dd", FIELD_DOCS[k]));
        d.appendChild(row);
      });
      det.appendChild(d);
      section.appendChild(det);
    });
  }

  /* ---------- API ---------- */

  /* Bewusst ohne jede Zwischenspeicherung: die Konsole soll das zeigen,
     was der Dienst gerade geantwortet hat, nicht was er einmal sagte. */
  function api(path, init) {
    var started = performance.now();
    var opts = { cache: "no-store", headers: { Accept: "application/json" } };
    if (init) {
      Object.keys(init).forEach(function (k) { opts[k] = init[k]; });
    }
    return fetch(path, opts).then(function (res) {
      var ms = Math.round(performance.now() - started);
      return res.text().then(function (raw) {
        var body = null;
        try { body = JSON.parse(raw); } catch (e) { body = null; }
        return { ok: res.ok, status: res.status, body: body, raw: raw, ms: ms };
      });
    }, function (err) {
      if (err && err.name === "AbortError") {
        // Bewusst KEIN Fehlerobjekt: ein Abbruch ist die Folge einer neueren
        // Eingabe, nie ein Zustand, den der Nutzer sehen soll.
        return { aborted: true, ok: false, status: 0, body: null, raw: "", ms: Math.round(performance.now() - started) };
      }
      return { ok: false, status: 0, body: null, raw: String(err), ms: Math.round(performance.now() - started) };
    });
  }

  function stamp(node, method, path, res) {
    node.textContent = method + " " + path + "  ·  HTTP " + res.status + "  ·  " + res.ms + " ms";
  }

  // Schreibt EINEN Satz in die Live-Region. Bewusst knapp: dieselbe Meldung
  // wird bei einer Tipp-Suche nach jeder Eingabepause erneut vorgelesen, also
  // ist alles ausser dem Ergebnis in einem Satz Lärm.
  var a11yStatus = byId("a11y-status");

  function announce(text) {
    if (a11yStatus) { a11yStatus.textContent = text; }
  }

  // role="alert" macht die Fehlermeldung zu einer Statusmeldung im Sinne von
  // WCAG 4.1.3: sie wird vorgelesen, sobald sie eingefügt wird, ohne dass sie
  // den Fokus an sich reisst. Jeder Fehlerpfad der vier Panels läuft hier
  // durch, deshalb genügt diese eine Stelle. Sie darf nirgends in einen
  // Container mit eigener Live-Rolle eingehängt werden — verschachtelte
  // Live-Regionen verhalten sich je nach Screenreader unterschiedlich.
  function errorBox(res) {
    var code = res.body && res.body.error ? res.body.error.code : "HTTP_" + res.status;
    var msg = res.body && res.body.error ? res.body.error.message : (res.raw || "ungültige oder leere JSON-Antwort");
    var box = el("div", "error", "Fehler " + code + ": " + msg);
    box.setAttribute("role", "alert");
    return box;
  }

  /* ---------- Panel 1: Suggest ---------- */

  var qInput = byId("suggest-q");
  var areaInput = byId("suggest-area");

  /* Gebiets-Lookup aus GET /v1/areas: füllt die Datalist mit "Germany (GER)"
     (value = Code) und eine Name->Code-Map, damit man im Feld sowohl "Germany"
     als auch "GER" tippen kann. Kein Blocker — schlägt der Aufruf fehl, bleibt
     das Feld ein normales Code-Freitextfeld. */
  var areaNameToCode = {}; // lowercased name AND "name (code)" -> code
  var areaCodes = {};      // lowercased code -> canonical code
  (function loadAreas() {
    var list = byId("areas");
    api("/v1/areas").then(function (res) {
      if (res.aborted) { return; }
      if (!res.ok || !res.body || !Array.isArray(res.body.areas)) { return; }
      var opts = [];
      res.body.areas.forEach(function (a) {
        areaCodes[a.code.toLowerCase()] = a.code;
        var label = a.name ? a.name + " (" + a.code + ")" : a.code;
        if (a.name) {
          areaNameToCode[a.name.toLowerCase()] = a.code;
          areaNameToCode[label.toLowerCase()] = a.code;
        }
        if (list) {
          var o = el("option");
          o.value = a.code;
          o.label = label;
          opts.push(o);
        }
      });
      if (list) { list.replaceChildren.apply(list, opts); }
    });
  }());

  /* Löst eine Eingabe auf einen Gebietscode auf: ein bekannter Code bleibt (in
     kanonischer Schreibweise), ein bekannter Name (oder "Name (CODE)") wird zum
     Code; alles andere bleibt unverändert und geht so an den Server (der die
     Aliase DE/AT/CH und rohe Codes selbst auflöst). */
  function resolveArea(input) {
    var key = input.toLowerCase();
    if (areaCodes[key]) { return areaCodes[key]; }
    if (areaNameToCode[key]) { return areaNameToCode[key]; }
    return input;
  }
  var limitInput = byId("suggest-limit");
  var suggestURL = byId("suggest-url");
  var suggestBackbone = byId("suggest-backbone");
  var suggestSpace = byId("suggest-space");
  var suggestSummary = byId("suggest-summary");
  var suggestBody = byId("suggest-body");

  var EXPECTED_RANKS = ["FAMILY", "GENUS", "SPECIES", "SUBSPECIES"];
  var suggestSeq = 0;
  var suggestTimer = null;
  var suggestAbort = null;

  // Bricht einen noch laufenden Suggest-Request ab. Wird beim TASTENDRUCK
  // aufgerufen, nicht erst wenn der Debounce-Timer feuert: die eine
  // SQLite-Verbindung des Servers wird so sofort frei, statt jede
  // Eingabepause als vollen Query auslaufen zu lassen.
  function abortSuggest() {
    if (suggestAbort !== null) { suggestAbort.abort(); suggestAbort = null; }
  }

  function rankMix(results) {
    var counts = {};
    EXPECTED_RANKS.forEach(function (r) { counts[r] = 0; });
    results.forEach(function (item) {
      var r = item.rank || "?";
      counts[r] = (counts[r] || 0) + 1;
    });
    return counts;
  }

  function renderRankMix(counts) {
    var line = el("div", "line");
    line.appendChild(el("span", null, "Rangmix: "));
    var keys = Object.keys(counts);
    keys.forEach(function (k, i) {
      if (i > 0) { line.appendChild(el("span", null, "  ·  ")); }
      line.appendChild(el("span", counts[k] === 0 ? "zero" : null, k + " " + counts[k]));
    });
    return line;
  }

  function renderSuggest(q, res) {
    suggestSummary.replaceChildren();
    suggestBody.replaceChildren();

    if (!res.ok || !res.body) {
      suggestSummary.appendChild(errorBox(res));
      return;
    }

    var results = (res.body && res.body.results) || [];
    if (results.length === 0) {
      suggestSummary.appendChild(el("div", "line", "Keine Treffer."));
      announce("Keine Treffer.");
      return;
    }
    announce(results.length === 1 ? "1 Treffer." : results.length + " Treffer.");

    var needle = fold(q);
    var prefixHits = 0;
    results.forEach(function (item) {
      if (fold(item.canonical).indexOf(needle) === 0) { prefixHits += 1; }
    });
    var missed = results.length - prefixHits;

    var prefixLine = el("div", "line " + (missed > 0 ? "bad" : "ok"));
    prefixLine.textContent = "Präfix „" + q + "“: " + prefixHits + " von " + results.length +
      " Treffern beginnen damit, " + missed + " nicht.";
    suggestSummary.appendChild(prefixLine);

    var first = results[0];
    var firstIsPrefix = fold(first.canonical).indexOf(needle) === 0;
    var firstLine = el("div", "line " + (firstIsPrefix ? "ok" : "bad"));
    firstLine.textContent = "Position 1: " + first.canonical + " (" + first.rank + ")" +
      (firstIsPrefix ? "" : " — beginnt nicht mit dem getippten Präfix.");
    suggestSummary.appendChild(firstLine);

    suggestSummary.appendChild(renderRankMix(rankMix(results)));

    results.forEach(function (item, i) {
      var isPrefix = fold(item.canonical).indexOf(needle) === 0;
      var tr = el("tr", "hit" + (isPrefix ? "" : " noprefix"));
      tr.appendChild(cell("td", i + 1, "num"));
      var nameCell = cell("td", item.display || item.canonical, "name");
      if (item.aggregate) {
        nameCell.appendChild(document.createTextNode(" "));
        nameCell.appendChild(badge("agg.", "neutral"));
      }
      tr.appendChild(nameCell);
      tr.appendChild(secTd(item.sec, item.concept_id));

      // Nur befüllt, wenn ein Namensraum gewählt ist. Die LEERE Zelle ist die
      // eigentliche Aussage: dieser Kandidat lässt sich dort nicht benennen,
      // taugt also nicht zur Weiterverarbeitung in dem Raum.
      var spaceTd = el("td", "name");
      if (suggestSpace && suggestSpace.value) {
        if (item.target_space_name) {
          spaceTd.textContent = item.target_space_name;
        } else {
          spaceTd.appendChild(badge("kein Name", "neutral"));
        }
      } else {
        spaceTd.textContent = "";
      }
      tr.appendChild(spaceTd);

      tr.appendChild(cell("td", item.rank));

      var acc = el("td");
      acc.appendChild(item.status === "ACCEPTED" ? badge("ja", "ok") : badge(item.status || "?", "warn"));
      tr.appendChild(acc);

      var area = el("td");
      area.appendChild(item.in_area ? badge("ja", "ok") : badge("keine Angabe", "neutral"));
      tr.appendChild(area);

      tr.appendChild(cell("td", num(item.score, 3), "num"));

      var pfx = el("td");
      pfx.appendChild(isPrefix ? badge("ja", "ok") : badge("nein", "bad"));
      tr.appendChild(pfx);

      tr.addEventListener("click", function () {
        Array.prototype.forEach.call(suggestBody.children, function (row) { row.classList.remove("selected"); });
        tr.classList.add("selected");
        showConcept(item.concept_id);
      });
      suggestBody.appendChild(tr);
    });
  }

  function runSuggest() {
    var q = qInput.value.trim();
    if (q === "") {
      suggestURL.textContent = "";
      suggestSummary.replaceChildren();
      suggestBody.replaceChildren();
      suggestSeq += 1; // Leer-Query verwirft auch jede noch offene Antwort
      return;
    }
    var params = new URLSearchParams();
    params.set("q", q);
    var area = resolveArea(areaInput.value.trim());
    if (area !== "") { params.set("area", area); }
    var limit = limitInput.value.trim();
    if (limit !== "") { params.set("limit", limit); }
    if (suggestBackbone && suggestBackbone.value) { params.set("entry_backbone", suggestBackbone.value); }
    if (suggestSpace && suggestSpace.value) { params.set("target_space", suggestSpace.value); }

    var path = "/v1/suggest?" + params.toString();
    var seq = suggestSeq + 1;
    suggestSeq = seq;
    suggestURL.textContent = "GET " + path + " …";

    suggestAbort = new AbortController();
    api(path, { signal: suggestAbort.signal }).then(function (res) {
      if (res.aborted) { return; }
      if (seq !== suggestSeq) { return; }
      stamp(suggestURL, "GET", path, res);
      renderSuggest(q, res);
    });
  }

  function scheduleSuggest() {
    // Abbruch SCHON beim Tastendruck, nicht erst beim Timer: der Entlastungs-
    // Effekt für die eine SQLite-Verbindung soll sofort greifen.
    abortSuggest();
    if (suggestTimer !== null) { clearTimeout(suggestTimer); }
    suggestTimer = setTimeout(runSuggest, 250);
  }

  qInput.addEventListener("input", scheduleSuggest);
  areaInput.addEventListener("input", scheduleSuggest);
  limitInput.addEventListener("input", scheduleSuggest);

  /* ---------- Panel 2: Konzept ---------- */

  var conceptOut = byId("concept-out");
  var currentConceptID = null;

  function renderXrefs(xrefs) {
    var box = el("div");
    box.appendChild(el("h3", null, "Xrefs"));
    var keys = Object.keys(xrefs || {}).sort();
    if (keys.length === 0) {
      box.appendChild(el("p", "empty", "Keine Xrefs erfasst."));
      return box;
    }
    var t = table(["Autorität", "Anzahl", "IDs"]);
    var body = t.tBodies[0];
    keys.forEach(function (authority) {
      var ids = xrefs[authority] || [];
      var tr = el("tr");
      tr.appendChild(cell("td", authority));
      var n = el("td", "num");
      n.appendChild(ids.length > 1 ? badge(String(ids.length), "warn") : el("span", null, String(ids.length)));
      tr.appendChild(n);
      tr.appendChild(cell("td", ids.join(", "), "mono"));
      body.appendChild(tr);
    });
    box.appendChild(scroller(t));
    return box;
  }

  function renderClassification(chain) {
    var box = el("div");
    box.appendChild(el("h3", null, "Elternkette (Wurzel zuerst)"));
    if (!chain || chain.length === 0) {
      box.appendChild(el("p", "empty", "Keine Elternkette erfasst."));
      return box;
    }
    box.appendChild(el("p", null, chain.map(function (c) {
      return c.canonical + " [" + c.rank + "]";
    }).join(" › ")));
    return box;
  }

  function renderConceptSynonyms(syns) {
    var box = el("div");
    box.appendChild(el("h3", null, "Synonyme (" + (syns ? syns.length : 0) + ")"));
    if (!syns || syns.length === 0) {
      box.appendChild(el("p", "empty", "Keine Synonyme erfasst."));
      return box;
    }
    var t = table(["Name", "Autor", "Typisierung"]);
    var body = t.tBodies[0];
    syns.forEach(function (s) {
      var tr = el("tr");
      tr.appendChild(cell("td", s.canonical, "name"));
      tr.appendChild(cell("td", s.authorship || "–"));
      var typ = el("td");
      if (s.homotypic === true) {
        typ.appendChild(badge("homotypisch", "ok"));
      } else if (s.homotypic === false) {
        typ.appendChild(badge("heterotypisch", "neutral"));
      } else {
        typ.appendChild(badge("unbekannt", "neutral"));
      }
      tr.appendChild(typ);
      body.appendChild(tr);
    });
    box.appendChild(scroller(t));
    return box;
  }

  function renderDistribution(dists) {
    var box = el("div");
    box.appendChild(el("h3", null, "Verbreitung"));
    if (!dists || dists.length === 0) {
      box.appendChild(el("p", "empty", "Keine Gebiete erfasst."));
      return box;
    }
    box.appendChild(el("p", "mono", dists.map(function (d) {
      return d.area_scheme + ":" + d.area_code;
    }).join(", ")));
    return box;
  }

  function renderPublicationSynonyms(res) {
    var box = el("div");
    box.appendChild(el("h3", null, "Synonyme, publikationsrelevant (relevance=publication)"));
    if (!res.ok || !res.body) {
      box.appendChild(errorBox(res));
      return box;
    }
    var body = res.body || {};
    var s = body.summary || {};
    box.appendChild(el("p", null,
      "gesamt " + (s.total || 0) + "  ·  publizierbar " + (s.publishable || 0) +
      "  ·  geliefert " + (s.returned || 0) + "  ·  abgeschnitten " + (s.truncated || 0)));
    if (body.ordering) { box.appendChild(el("p", "hint", "Sortierung: " + body.ordering)); }

    var syns = body.synonyms || [];
    if (syns.length === 0) {
      box.appendChild(el("p", "empty", "Keine publikationsrelevanten Synonyme."));
      return box;
    }
    var t = table(["#", "Name", "Autor", ["Rang", "rank"], ["Typisierung", "typification"], ["Basionym", "basionym"], ["publizierbar", "publishable"], ["nom_status", "nom_status"], ["Begründung", "reason"]]);
    var tbody = t.tBodies[0];
    syns.forEach(function (d) {
      var tr = el("tr");
      tr.appendChild(cell("td", d.position, "num"));
      tr.appendChild(cell("td", d.canonical, "name"));
      tr.appendChild(cell("td", d.authorship || "–"));
      tr.appendChild(cell("td", d.rank + (d.rank_verbatim ? " (" + d.rank_verbatim + ")" : "")));
      tr.appendChild(cell("td", d.typification));
      var bas = el("td");
      bas.appendChild(d.is_basionym ? badge("ja", "ok") : badge("nein", "neutral"));
      tr.appendChild(bas);
      var pub = el("td");
      pub.appendChild(d.publishable ? badge("ja", "ok") : badge("nein", "bad"));
      tr.appendChild(pub);
      tr.appendChild(cell("td", (d.nom_status || "–") + " / " + d.nom_status_judgement));
      tr.appendChild(cell("td", d.reason));
      tbody.appendChild(tr);
    });
    box.appendChild(scroller(t));
    return box;
  }

  function showConcept(id) {
    currentConceptID = id;
    translateBtn.disabled = false;
    conceptOut.replaceChildren(el("p", "busy", "lade …"));
    translateOut.replaceChildren(el("p", "empty", "Noch nicht übersetzt."));

    var base = "/v1/concept/" + encodeURIComponent(id);
    Promise.all([
      api(base),
      api(base + "/synonyms?relevance=publication")
    ]).then(function (all) {
      if (all[0].aborted || all[1].aborted) { return; }
      if (currentConceptID !== id) { return; }
      var conceptRes = all[0];
      var out = el("div");
      out.appendChild(el("p", "url", "GET " + base + "  ·  HTTP " + conceptRes.status + "  ·  " + conceptRes.ms + " ms"));

      if (!conceptRes.ok || !conceptRes.body) {
        out.appendChild(errorBox(conceptRes));
        conceptOut.replaceChildren(out);
        return;
      }
      var c = conceptRes.body || {};
      out.appendChild(dl([
        ["concept_id", c.concept_id],
        ["Anzeige", c.display],
        ["Kanonisch", c.canonical],
        ["Rang", c.rank + (c.rank_verbatim ? " (verbatim: " + c.rank_verbatim + ")" : ""), "rank"],
        ["Status", c.status],
        ["sec.", c.sec && c.sec.title ? c.sec.title : secSource(c.concept_id), "sec"],
        ["Backbone", c.backbone ? c.backbone.id + " @ " + c.backbone.version : "", "backbone"]
      ]));
      out.appendChild(renderClassification(c.parent_chain));
      out.appendChild(renderXrefs(c.xrefs));
      out.appendChild(renderDistribution(c.distribution));
      out.appendChild(renderConceptSynonyms(c.synonyms));
      out.appendChild(renderPublicationSynonyms(all[1]));
      conceptOut.replaceChildren(out);
    });
  }

  /* ---------- Panel 3: Match ---------- */

  var matchInput = byId("match-input");
  var matchURL = byId("match-url");
  var matchBackbone = byId("match-backbone");
  var matchSec = byId("match-sec");
  var matchSpace = byId("match-space");
  var matchOut = byId("match-out");
  var matchSeq = 0;
  var matchAbort = null;

  function renderMatch(lines, res) {
    matchOut.replaceChildren();
    if (!res.ok || !res.body) {
      matchOut.appendChild(errorBox(res));
      return;
    }
    var results = (res.body && res.body.results) || [];
    var t = table(["#", "Verbatim", ["Einstufung", "match_type"], ["Konfidenz", "confidence"], ["Prüfung", "requires_review"], "concept_id", "Kandidaten", "Notiz"]);
    var tbody = t.tBodies[0];
    results.forEach(function (r, i) {
      var tr = el("tr");
      tr.appendChild(cell("td", r.id || String(i + 1), "num"));
      tr.appendChild(cell("td", lines[i] === undefined ? "" : lines[i], "name"));
      var mt = el("td");
      mt.appendChild(badge(r.match_type, r.match_type === "unresolvable" ? "bad" : "ok"));
      tr.appendChild(mt);
      tr.appendChild(cell("td", num(r.confidence, 2), "num"));
      var rev = el("td");
      rev.appendChild(r.requires_review ? badge("Review nötig", "bad") : badge("ok", "ok"));
      tr.appendChild(rev);

      var idCell = el("td", "mono");
      if (r.concept_id) {
        var btn = el("button", null, r.concept_id);
        btn.type = "button";
        btn.addEventListener("click", function () { showConcept(r.concept_id); });
        idCell.appendChild(btn);
      } else {
        idCell.textContent = "–";
      }
      tr.appendChild(idCell);
      tr.appendChild(cell("td", (r.candidates || []).join(", ") || "–", "mono"));
      tr.appendChild(cell("td", r.note || "–"));
      tbody.appendChild(tr);
    });
    matchOut.appendChild(scroller(t));
  }

  byId("match-run").addEventListener("click", function () {
    var lines = matchInput.value.split("\n").map(function (s) { return s.trim(); })
      .filter(function (s) { return s !== ""; });
    if (lines.length === 0) {
      matchOut.replaceChildren(el("p", "empty", "Keine Namen eingegeben."));
      matchURL.textContent = "";
      return;
    }
    var payload = { names: lines.map(function (v, i) { return { id: String(i + 1), verbatim: v }; }) };
    if (matchBackbone && matchBackbone.value) { payload.entry_backbone = matchBackbone.value; }
    var msec = matchSec ? matchSec.value.trim() : "";
    if (msec !== "") { payload.entry_sec = msec; }
    if (matchSpace && matchSpace.value) { payload.target_space = matchSpace.value; }
    matchURL.textContent = "POST /v1/match …";
    if (matchAbort !== null) { matchAbort.abort(); }
    matchAbort = new AbortController();
    var seq = matchSeq + 1;
    matchSeq = seq;
    api("/v1/match", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(payload),
      signal: matchAbort.signal
    }).then(function (res) {
      if (res.aborted) { return; }
      if (seq !== matchSeq) { return; }
      stamp(matchURL, "POST", "/v1/match", res);
      renderMatch(lines, res);
    });
  });

  /* ---------- Panel 4: Translate ---------- */

  var translateTarget = byId("translate-target");
  var translateNames = byId("translate-names");
  var translateBtn = byId("translate-run");
  var translateURL = byId("translate-url");
  var translateOut = byId("translate-out");
  var translateSeq = 0;
  var translateAbort = null;

  function noRelationStatement(target, body) {
    var box = el("div", "statement");
    box.appendChild(el("strong", null, "keine Relation erfasst"));
    box.appendChild(el("p", null,
      "Der Dienst hat mit result = no_relation_recorded geantwortet: zwischen diesem Konzept und dem Raum „" +
      target + "“ ist keine Relation hinterlegt."));
    box.appendChild(el("p", null,
      "Das ist eine Aussage über den Datenbestand, kein Fehler und kein leeres Ergebnis. " +
      "Ein Name ist über sec.-Räume hinweg konstruktionsbedingt mehrdeutig; ohne erfasste Relation " +
      "wäre jede Antwort geraten."));
    if (body && body.note) { box.appendChild(el("p", null, "Notiz des Dienstes: " + body.note)); }
    return box;
  }

  function renderCandidates(cands) {
    var t = table(["Name", ["sec.", "sec"], "gespeicherte Relation", "Richtung", "aus Quellsicht", "Aussage", "Gleichheit", "Hops"]);
    var tbody = t.tBodies[0];
    cands.forEach(function (c) {
      var tr = el("tr");
      tr.appendChild(cell("td", c.canonical + (c.authorship ? " " + c.authorship : ""), "name"));
      tr.appendChild(secTd(c.sec, c.concept_id));
      tr.appendChild(cell("td", c.stored_relation));
      tr.appendChild(cell("td", c.direction));
      var rel = el("td");
      if (c.relation_from_source === null) {
        rel.appendChild(badge("kein gültiger Umkehrschluss", "warn"));
      } else {
        rel.appendChild(el("span", null, c.relation_from_source));
      }
      tr.appendChild(rel);
      tr.appendChild(cell("td", c.statement
        ? c.statement.from + " " + c.statement.relation + " " + c.statement.to
        : "–", "mono"));
      var eq = el("td");
      eq.appendChild(c.is_equality ? badge("ja", "ok") : badge("nein", "neutral"));
      tr.appendChild(eq);
      tr.appendChild(cell("td", c.hops, "num"));
      tbody.appendChild(tr);
    });
    return scroller(t);
  }

  function renderTranslate(target, res) {
    translateOut.replaceChildren();
    if (!res.ok || !res.body) {
      translateOut.appendChild(errorBox(res));
      return;
    }
    var body = res.body || {};
    var head = el("div");
    head.appendChild(el("p", null,
      "Zielraum " + (body.target_space ? (body.target_space.title || body.target_space.id) : target) +
      "  ·  max_hops " + body.max_hops +
      "  ·  " + (body.requires_review ? "Prüfung nötig" : "keine Prüfung nötig")));
    translateOut.appendChild(head);

    if (body.result === "no_relation_recorded") {
      translateOut.appendChild(noRelationStatement(target, body));
    } else {
      var cands = body.candidates || [];
      if (cands.length === 0) {
        translateOut.appendChild(el("div", "statement", "result = " + body.result + ", aber keine Kandidaten geliefert."));
      } else {
        translateOut.appendChild(renderCandidates(cands));
      }
    }

    var names = body.unrelated_name_candidates || [];
    if (names.length > 0) {
      translateOut.appendChild(el("h3", null, "Namensgleiche Konzepte — NICHT relational"));
      translateOut.appendChild(el("p", "hint",
        "Nur zur Sichtprüfung: gleicher Name im Zielraum, ohne erfasste Relation. Keine Übersetzung."));
      var t = table(["Name", ["sec.", "sec"], ["Rang", "rank"]]);
      var tbody = t.tBodies[0];
      names.forEach(function (n) {
        var tr = el("tr");
        tr.appendChild(cell("td", n.canonical + (n.authorship ? " " + n.authorship : ""), "name"));
        tr.appendChild(secTd(n.sec, n.concept_id));
        tr.appendChild(cell("td", n.rank));
        tbody.appendChild(tr);
      });
      translateOut.appendChild(scroller(t));
    }
  }

  translateBtn.addEventListener("click", function () {
    if (currentConceptID === null) { return; }
    var target = translateTarget.value.trim();
    if (target === "") {
      translateOut.replaceChildren(el("div", "statement", "Bitte einen Ziel-sec.-Raum angeben."));
      return;
    }
    var payload = {
      concept_id: currentConceptID,
      target_space: target,
      include_name_candidates: translateNames.checked
    };
    translateURL.textContent = "POST /v1/translate …";
    if (translateAbort !== null) { translateAbort.abort(); }
    translateAbort = new AbortController();
    var seq = translateSeq + 1;
    translateSeq = seq;
    api("/v1/translate", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(payload),
      signal: translateAbort.signal
    }).then(function (res) {
      if (res.aborted) { return; }
      if (seq !== translateSeq) { return; }
      stamp(translateURL, "POST", "/v1/translate", res);
      renderTranslate(target, res);
    });
  });

  /* Ziel-sec.-Räume aus GET /v1/sec in die datalist füllen, damit
     target_space eine Auswahl statt eines Freitext-Ratens bietet: ein
     geratener Raum ist sonst von einem leeren Ergebnis nicht zu
     unterscheiden. Kein Blocker — schlägt der Aufruf fehl, bleibt das Feld
     ein normales Freitextfeld. */
  (function loadSecSpaces() {
    var list = byId("sec-spaces");
    if (!list) { return; }
    api("/v1/sec").then(function (res) {
      if (res.aborted) { return; }
      if (!res.ok || !res.body || !Array.isArray(res.body.sec_references)) { return; }
      var opts = res.body.sec_references.map(function (s) {
        var o = el("option");
        o.value = s.id;
        if (s.title) { o.label = s.title; }
        return o;
      });
      list.replaceChildren.apply(list, opts);
    });
  }());

  // Die beiden Auswahlfelder werden aus dem Index gefüllt, nicht fest
  // verdrahtet: welche Backbones und Namensräume vorliegen, entscheidet das
  // Manifest des jeweiligen Deployments. Schlägt der Abruf fehl, bleibt die
  // Auswahl bei "Alle"/"keiner" — also beim bisherigen Verhalten.
  function fillSelect(sel, items, valueOf, labelOf) {
    if (!sel) { return; }
    items.forEach(function (it) {
      var o = el("option");
      o.value = valueOf(it);
      o.textContent = labelOf(it);
      sel.appendChild(o);
    });
  }

  (function loadCatalog() {
    api("/v1/backbones").then(function (res) {
      if (res.aborted) { return; }
      if (!res.ok || !res.body || !Array.isArray(res.body.backbones)) { return; }
      [suggestBackbone, matchBackbone].forEach(function (sel) {
        fillSelect(sel, res.body.backbones,
          function (b) { return b.id; },
          function (b) { return b.id + " (" + b.version + ")"; });
      });
    });
    api("/v1/spaces").then(function (res) {
      if (res.aborted) { return; }
      if (!res.ok || !res.body || !Array.isArray(res.body.spaces)) { return; }
      [suggestSpace, matchSpace].forEach(function (sel) {
        fillSelect(sel, res.body.spaces,
          function (sp) { return sp.id; },
          function (sp) { return sp.id + " (" + sp.version + ")"; });
      });
    });
  }());

  // Feld-Hinweise (Tooltip am ⓘ) und aufklappbare Legenden aufbauen.
  enhanceDocs(document);
  buildLegends();

  // Die im Markup fest stehenden Scroll-Container (die dynamisch erzeugten
  // meldet scroller() selbst an).
  Array.prototype.forEach.call(document.querySelectorAll(".scroll"), watchScroller);
}());
