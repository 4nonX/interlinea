package docxtest

// Hofmann builds the sample residential lease used by the prototype and the
// tests: a title, numbered sections, a rent table, inline bold and italics,
// two footnotes (one in the middle of a sentence), an external link, a
// signature table with repeated cells, a header and a footer with page
// number fields.
func Hofmann() []byte {
	body := P("Title", R("Mietvertrag über Wohnraum")) +
		P("", R("Zwischen "), B("Herrn Jonas Hofmann"), R(" (Vermieter) und "), B("Frau Lea Kraus"), R(" (Mieterin) wird folgender Mietvertrag geschlossen:")) +
		P("Heading1", R("§ 1 Mietsache")) +
		P("", R("Vermietet wird die Wohnung im zweiten Obergeschoss des Hauses Lindenstraße 12, 80331 München. Die Mietsache besteht aus drei Zimmern, einer Küche, einem Bad und einem Kellerraum.")) +
		P("Heading1", R("§ 2 Mietzeit")) +
		P("", R("Das Mietverhältnis beginnt am 1. Januar 2027 und läuft auf unbestimmte Zeit. Die ordentliche Kündigung richtet sich nach den gesetzlichen Vorschriften."), FootnoteRef(1)) +
		P("Heading1", R("§ 3 Miete und Betriebskosten")) +
		P("", R("Die monatliche Miete beträgt "), B("1.250,00 EUR"), R(" zuzüglich einer Vorauszahlung auf die Betriebskosten in Höhe von "), B("220,00 EUR"), R(".")) +
		Table([]int{5200, 2600},
			[]string{P("", R("Nettokaltmiete")), Right(R("1.250,00 EUR"))},
			[]string{P("", R("Vorauszahlung Betriebskosten")), Right(R("220,00 EUR"))},
			[]string{P("", B("Gesamtmiete")), Right(B("1.470,00 EUR"))},
		) +
		P("", R("Die Miete ist monatlich im Voraus, spätestens am dritten Werktag eines Monats, auf das Konto des Vermieters zu zahlen. Über die Betriebskosten wird jährlich abgerechnet.")) +
		P("Heading1", R("§ 4 Kaution")) +
		P("", R("Der Mieter leistet eine Kaution in Höhe von drei Nettokaltmieten. Die Kaution ist zu Beginn des Mietverhältnisses fällig und wird nach Beendigung des Mietverhältnisses zurückgezahlt.")) +
		P("Heading1", R("§ 5 Pflichten des Mieters")) +
		P("", R("Der Mieter übernimmt die Schönheitsreparaturen auf eigene Kosten. Eine Untervermietung bedarf der "), I("vorherigen schriftlichen"), R(" Zustimmung des Vermieters.")) +
		P("", R("Das Recht zur Mietminderung"), FootnoteRef(2), R(" bleibt unberührt. Der Vermieter kann Räumungsklage erheben, wenn der Mieter mit zwei Monatsmieten in Verzug ist.")) +
		P("Heading1", R("§ 6 Schlussbestimmungen")) +
		P("", R("Änderungen und Ergänzungen dieses Vertrages bedürfen der Schriftform. Sollte eine Bestimmung dieses Vertrages unwirksam sein, bleibt die Wirksamkeit der übrigen Bestimmungen unberührt.")) +
		P("", R("Die genannten Gesetze sind unter "), Link("rIdLink1", "Gesetze im Internet"), R(" abrufbar.")) +
		P("", R("")) +
		Table([]int{3900, 3900},
			[]string{P("", R("Ort, Datum")), P("", R("Ort, Datum"))},
			[]string{P("", R("Unterschrift Vermieter")), P("", R("Unterschrift Mieter"))},
		)
	return Document(Parts{
		Body: body,
		Footnotes: []string{
			Footnote("Siehe §§ 573 ff. BGB."),
			Footnote("Siehe § 536 BGB."),
		},
		Header: P("Header", R("Mietvertrag Hofmann, Entwurf vom 16. November 2026")),
		Footer: P("Footer", R("Seite "), Field("PAGE", "1"), R(" von "), Field("NUMPAGES", "2")),
		Links:  map[string]string{"rIdLink1": "https://www.gesetze-im-internet.de/bgb/"},
		Lang:   "de-DE",
	})
}
