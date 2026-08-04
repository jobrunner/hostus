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

  function table(headers) {
    var t = el("table");
    var thead = el("thead");
    var tr = el("tr");
    headers.forEach(function (h) { tr.appendChild(cell("th", h)); });
    thead.appendChild(tr);
    t.appendChild(thead);
    t.appendChild(el("tbody"));
    return t;
  }

  function scroller(node) {
    var d = el("div", "scroll");
    d.appendChild(node);
    return d;
  }

  function badge(text, kind) {
    return el("span", "badge " + kind, text);
  }

  function dl(pairs) {
    var d = el("dl", "kv");
    pairs.forEach(function (p) {
      if (p[1] === undefined || p[1] === null || p[1] === "") { return; }
      var row = el("div");
      row.appendChild(cell("dt", p[0]));
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
      return { ok: false, status: 0, body: null, raw: String(err), ms: Math.round(performance.now() - started) };
    });
  }

  function stamp(node, method, path, res) {
    node.textContent = method + " " + path + "  ·  HTTP " + res.status + "  ·  " + res.ms + " ms";
  }

  function errorBox(res) {
    var code = res.body && res.body.error ? res.body.error.code : "HTTP_" + res.status;
    var msg = res.body && res.body.error ? res.body.error.message : (res.raw || "keine Antwort");
    return el("div", "error", "Fehler " + code + ": " + msg);
  }

  /* ---------- Panel 1: Suggest ---------- */

  var qInput = byId("suggest-q");
  var areaInput = byId("suggest-area");
  var limitInput = byId("suggest-limit");
  var suggestURL = byId("suggest-url");
  var suggestSummary = byId("suggest-summary");
  var suggestBody = byId("suggest-body");

  var EXPECTED_RANKS = ["FAMILY", "GENUS", "SPECIES", "SUBSPECIES"];
  var suggestSeq = 0;
  var suggestTimer = null;

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

    if (!res.ok) {
      suggestSummary.appendChild(errorBox(res));
      return;
    }

    var results = (res.body && res.body.results) || [];
    if (results.length === 0) {
      suggestSummary.appendChild(el("div", "line", "Keine Treffer."));
      return;
    }

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
      tr.appendChild(cell("td", item.display || item.canonical, "name"));
      tr.appendChild(cell("td", item.rank));

      var acc = el("td");
      acc.appendChild(item.status === "ACCEPTED" ? badge("ja", "ok") : badge(item.status || "?", "warn"));
      tr.appendChild(acc);

      var area = el("td");
      area.appendChild(item.in_area ? badge("ja", "ok") : badge("nein", "neutral"));
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
      return;
    }
    var params = new URLSearchParams();
    params.set("q", q);
    var area = areaInput.value.trim();
    if (area !== "") { params.set("area", area); }
    var limit = limitInput.value.trim();
    if (limit !== "") { params.set("limit", limit); }

    var path = "/v1/suggest?" + params.toString();
    var seq = suggestSeq + 1;
    suggestSeq = seq;
    suggestURL.textContent = "GET " + path + " …";

    api(path).then(function (res) {
      if (seq !== suggestSeq) { return; }
      stamp(suggestURL, "GET", path, res);
      renderSuggest(q, res);
    });
  }

  function scheduleSuggest() {
    if (suggestTimer !== null) { clearTimeout(suggestTimer); }
    suggestTimer = setTimeout(runSuggest, 150);
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
    box.appendChild(el("h3", null, "Klassifikation (Wurzel zuerst)"));
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

  function renderTraits(res) {
    var box = el("div");
    box.appendChild(el("h3", null, "Traits"));
    if (!res.ok) {
      box.appendChild(errorBox(res));
      return box;
    }
    var sets = (res.body && res.body.traits) || [];
    if (sets.length === 0) {
      box.appendChild(el("p", "empty", "Keine Indikatorwerte erfasst."));
      return box;
    }
    sets.forEach(function (set) {
      var head = set.vocab + " " + set.vocab_version + (set.taxonomy ? "  ·  " + set.taxonomy : "");
      box.appendChild(el("p", null, head));
      var t = table(["Dim", "Wert", "Skala", "Nischenbreite", "n_systems", "Auflösung"]);
      var body = t.tBodies[0];
      (set.values || []).forEach(function (v) {
        var tr = el("tr");
        tr.appendChild(cell("td", v.dim));
        tr.appendChild(cell("td", num(v.value, 3), "num"));
        var scale = v.scale
          ? num(v.scale.min, 1) + "–" + num(v.scale.max, 1) + (v.scale.normalized ? " (norm.)" : "")
          : "–";
        tr.appendChild(cell("td", scale));
        tr.appendChild(cell("td", v.niche_width === undefined ? "–" : num(v.niche_width, 3), "num"));
        tr.appendChild(cell("td", v.n_systems === undefined ? "–" : String(v.n_systems), "num"));
        var resol = el("td");
        if (v.resolution) {
          resol.appendChild(badge(v.resolution, "warn"));
        } else {
          resol.appendChild(el("span", null, "exakt"));
        }
        tr.appendChild(resol);
        body.appendChild(tr);
      });
      box.appendChild(scroller(t));
    });
    return box;
  }

  function renderPublicationSynonyms(res) {
    var box = el("div");
    box.appendChild(el("h3", null, "Synonyme, publikationsrelevant (relevance=publication)"));
    if (!res.ok) {
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
    var t = table(["#", "Name", "Autor", "Rang", "Typisierung", "Basionym", "publizierbar", "nom_status", "Begründung"]);
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
      api(base + "/traits"),
      api(base + "/synonyms?relevance=publication")
    ]).then(function (all) {
      if (currentConceptID !== id) { return; }
      var conceptRes = all[0];
      var out = el("div");
      out.appendChild(el("p", "url", "GET " + base + "  ·  HTTP " + conceptRes.status + "  ·  " + conceptRes.ms + " ms"));

      if (!conceptRes.ok) {
        out.appendChild(errorBox(conceptRes));
        conceptOut.replaceChildren(out);
        return;
      }
      var c = conceptRes.body || {};
      out.appendChild(dl([
        ["concept_id", c.concept_id],
        ["Anzeige", c.display],
        ["Kanonisch", c.canonical],
        ["Rang", c.rank + (c.rank_verbatim ? " (verbatim: " + c.rank_verbatim + ")" : "")],
        ["Status", c.status],
        ["Backbone", c.backbone ? c.backbone.id + " @ " + c.backbone.version : ""]
      ]));
      out.appendChild(renderClassification(c.classification));
      out.appendChild(renderXrefs(c.xrefs));
      out.appendChild(renderDistribution(c.distribution));
      out.appendChild(renderConceptSynonyms(c.synonyms));
      out.appendChild(renderTraits(all[1]));
      out.appendChild(renderPublicationSynonyms(all[2]));
      conceptOut.replaceChildren(out);
    });
  }

  /* ---------- Panel 3: Match ---------- */

  var matchInput = byId("match-input");
  var matchURL = byId("match-url");
  var matchOut = byId("match-out");

  function renderMatch(lines, res) {
    matchOut.replaceChildren();
    if (!res.ok) {
      matchOut.appendChild(errorBox(res));
      return;
    }
    var results = (res.body && res.body.results) || [];
    var t = table(["#", "Verbatim", "Einstufung", "Konfidenz", "Prüfung", "concept_id", "Kandidaten", "Notiz"]);
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
    matchURL.textContent = "POST /v1/match …";
    api("/v1/match", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(payload)
    }).then(function (res) {
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
    var t = table(["Name", "sec.", "gespeicherte Relation", "Richtung", "aus Quellsicht", "Aussage", "Gleichheit", "Hops"]);
    var tbody = t.tBodies[0];
    cands.forEach(function (c) {
      var tr = el("tr");
      tr.appendChild(cell("td", c.canonical + (c.authorship ? " " + c.authorship : ""), "name"));
      tr.appendChild(cell("td", c.sec ? c.sec.id : "–"));
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
    if (!res.ok) {
      translateOut.appendChild(errorBox(res));
      return;
    }
    var body = res.body || {};
    var head = el("div");
    head.appendChild(el("p", null,
      "Zielraum " + (body.target_space ? body.target_space.id : target) +
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
      var t = table(["Name", "sec.", "Rang"]);
      var tbody = t.tBodies[0];
      names.forEach(function (n) {
        var tr = el("tr");
        tr.appendChild(cell("td", n.canonical + (n.authorship ? " " + n.authorship : ""), "name"));
        tr.appendChild(cell("td", n.sec ? n.sec.id : "–"));
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
    api("/v1/translate", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(payload)
    }).then(function (res) {
      stamp(translateURL, "POST", "/v1/translate", res);
      renderTranslate(target, res);
    });
  });
}());
