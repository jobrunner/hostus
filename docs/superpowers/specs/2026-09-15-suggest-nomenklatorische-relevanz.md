# Spec: Illegitime Namen dürfen im Suggest nicht gleichrangig auftreten

Datum: 2026-09-15
Status: verabschiedet (User: „Wenn man ‚Inula hirta' über wcvp sucht, wird
aber immer noch Inula hirta (als Synonym) an zweiter Stelle angezeigt. Das
ist immer noch gruselig für einen Anwender." → Befund vorgelegt → „Ja, passt")

## Befund

Wer „Inula hirta" sucht, bekommt zwei Zeilen, die denselben getippten Namen
tragen — und die zweite führt zu einer völlig anderen Art. Für einen
Anwender sieht das aus, als könne sich der Dienst nicht entscheiden.

Die Daten sagen etwas anderes. WCVP führt beide Namen mit Status:

```
Inula hirta L.        nom_status: (leer)                          → Pentanema hirtum
Inula hirta Pollich   nom_status: ", nom. illeg. homonym. post."  → Pentanema britannica
```

`Inula hirta` Pollich ist ein **später illegitimer Homonym-Name** —
nomenklatorisch ungültig, und zwar genau deshalb, weil *Inula hirta* L.
bereits existierte. Die beiden Zeilen sind also **nicht gleichrangig**; die
zweite beruht auf einem Namen, den der Code selbst schon als unbrauchbar
einstuft.

**hostus kennt diese Information und wirft sie im Suggest weg.**
`domain.ClassifyNomStatus` klassifiziert den Status dieses Namens als
`disqualifying` (geprüft), und `GET /v1/concept/{id}/synonyms` wertet ihn
seit SP6 aus. Der Suggest-Pfad liest `name.nom_status` überhaupt nicht —
weder fürs Ranking noch für die Antwort.

Gemessen an der Produktions-DB (v3.4.0-alpha.0):

| Messung | Wert |
|---|---|
| Namen mit irgendeinem `nom_status` | 99.252 |
| davon disqualifizierte Synonym-Zeilen in WCVP | 89.836 |
| Schreibweisen, unter denen ein disqualifizierter **und** ein nicht-disqualifizierter Name existiert | **31.970** |

Die letzte Zeile ist die eigentliche Verwechslungslage: rund 32.000-mal steht
ein ungültiger Name neben einem brauchbaren gleicher Schreibweise.

Alle drei Werte sind mit `domain.ClassifyNomStatus` selbst gemessen, nicht mit
einer Token-Näherung — eine erste Schätzung über `LIKE '%illeg%'` lag um rund
20 % zu niedrig, weil sie `nom. nud.`, `not validly publ.`, `orth. var.`,
`pro syn.` und `nom. rej.` nicht mitzählte. Gezählt werden nur Namen, die an
einem Konzept hängen (nur solche können im Suggest auftauchen).

## Entscheidungen

1. **Neues Ranking-Kriterium, direkt hinter dem Exakt-Treffer:** Ein
   Kandidat, dessen Treffer-Name disqualifiziert ist, verliert gegen einen,
   dessen Treffer-Name es nicht ist. Neue Reihenfolge in
   `domain.RankSuggestions`:

   1. `ExactHit`
   2. **`MatchedNameDisqualified` (neu, false vor true)**
   3. `TargetSpaceHit`
   4. `PrefixHit`
   5. `InArea`
   6. Status `accepted`
   7. Rangordnung
   8. Score aufsteigend

   Position 2 ist bewusst **vor** `TargetSpaceHit`: Ob ein Name überhaupt
   gültig publiziert ist, ist grundsätzlicher als die Frage, in welchem
   Namensraum er auftaucht — und der gemeldete Fall tritt gerade ohne
   angefragten `target_space` auf, wo Kriterium 3 nichts unterscheidet.

2. **Nur `disqualifying` wertet ab.** `domain.ClassifyNomStatus` kennt vier
   Urteile. `absent` (nichts erfasst), `acceptable` (z. B. `nom. cons.`) und
   `unclassified` zählen alle als *nicht disqualifiziert*. Besonders
   `unclassified` darf **nicht** abwerten: Das sind Fälle wie
   `sensu auct.` oder `fossil name`, bei denen die bestehende Regeltabelle
   ausdrücklich ein Urteil zurückhält — Unsicherheit ist kein Mangel. Die
   Abwertung ist damit exakt so streng wie die bereits von einem Botaniker
   durchgesehene Regeltabelle, keine Zeile strenger.

3. **Kein Ausblenden, niemals.** Die Zeile bleibt in der Liste. Wer einen
   illegitimen Namen in alter Literatur findet, muss nachschlagen können,
   was daraus geworden ist — das ist eine Kernaufgabe dieses Dienstes und
   nicht verhandelbar. Abwerten und kennzeichnen, nicht verstecken.

4. **Die Antwort nennt den Status.** `matched_name` bekommt zwei Felder,
   benannt wie im Synonym-Endpunkt, damit ein Client nicht zwei Vokabulare
   lernen muss:
   - `nom_status` — der normalisierte Rohwert (`omitempty`), z. B.
     `"nom. illeg. homonym. post."`.
   - `nom_status_judgement` — **immer** gerendert, Werte `absent`,
     `acceptable`, `disqualifying`, `unclassified`. Auch hier gilt die
     Begründung aus `synonyms.go`: „nichts erfasst" ist nicht dasselbe wie
     „geprüft und sauber", deshalb wird `absent` ausdrücklich ausgesprochen
     statt weggelassen.

5. **Die Auswahl des Treffer-Namens bevorzugt den gültigen.** Trägt ein
   Konzept mehrere Namen, die die Anfrage erfüllen, entscheidet ab jetzt:
   exakt vor Präfix → **nicht disqualifiziert vor disqualifiziert** →
   `accepted` vor `synonym` → `canonical` → `authorship` → `id`. Sonst
   könnte ein Konzept über einen gültigen Namen in die Liste kommen und
   trotzdem mit einem ungültigen beschriftet und abgewertet werden.

6. **Konsole:** Die Spalte „Treffer-Name" kennzeichnet einen
   disqualifizierten Namen sichtbar (Badge im Stil der vorhandenen
   Badges), mit dem normalisierten Status als Tooltip. Der Zweck ist, dass
   die zweite Zeile im *Inula-hirta*-Fall selbsterklärend wird: nicht „noch
   ein Treffer", sondern „ungültiger Name, hier ist er hingewandert".

7. **Außerhalb dieses Scopes:** `POST /v1/match` und `POST /v1/translate`
   (dort ist die Auflösung ID- bzw. namensbasiert und hat eigene
   Vorrangregeln — eine Übertragung braucht eigene Messungen); der
   `relevance`-Filter des Synonym-Endpunkts bleibt unverändert; Suggest
   bekommt **keinen** Filter-Parameter für Legitimität (Entscheidung 3).

## Zielbild, messbar

`q=Inula hirta&entry_backbone=wcvp` (ohne `target_space` — genau der
gemeldete Fall):

- **erste Zeile** ist *Pentanema hirtum* (`matched_name` = `Inula hirta L.`,
  `nom_status_judgement: "absent"`),
- **zweite Zeile** ist *Pentanema britannica* mit
  `nom_status: "nom. illeg. homonym. post."` und
  `nom_status_judgement: "disqualifying"`, in der Konsole sichtbar
  gekennzeichnet,
- die Reihenfolge steht **ohne** `target_space` fest, nicht erst durch ihn.

## Projektweite Anforderungen

- Mutation-gated: `internal/domain`, `internal/adapters/http`, `cmd/hostus`
  (CI) und lokal `internal/adapters/sqlite` — je `Not covered: 0`.
  `make mutation` läuft seit PR #122 in einer Sandbox; im Arbeitsbaum
  starten ist nicht nötig und nicht erwünscht.
- Die Ranking-Änderung braucht einen Test, der gegen die ALTE Reihenfolge
  fehlschlägt (Kontroll-Assertion), und einen, der die Position des neuen
  Kriteriums **vor** `TargetSpaceHit` pinnt — beide neuen Felder müssen
  zwischen den Vergleichsobjekten variieren, sonst pinnt der Test nichts
  (dieser Fehler ist im Vorgänger-Branch schon einmal passiert).
- Antworten ohne einen disqualifizierten Treffer bleiben unverändert,
  abgesehen vom additiven `nom_status_judgement`.
- Latenz: `attachMatchedNames` liest eine Spalte mehr, keine neue Query.
  Gegen die Produktions-DB vorher/nachher messen und im Report nennen —
  interleaved, im Leerlauf, nicht während eines Mutationslaufs.
- OpenAPI codegeneriert, beide Kopien identisch; CHANGELOG unter
  `## [Unreleased]`; Conventional Commits; englische WHY-Kommentare mit den
  Messwerten dieser Spec.
