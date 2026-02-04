package connector

type sourceCitation struct {
	URL   string
	Title string
}

func extractURLCitation(annotation any) (sourceCitation, bool) {
	raw, ok := annotation.(map[string]any)
	if !ok {
		return sourceCitation{}, false
	}
	typ, _ := raw["type"].(string)
	if typ != "url_citation" {
		return sourceCitation{}, false
	}
	url, ok := readStringArg(raw, "url")
	if !ok {
		return sourceCitation{}, false
	}
	title, _ := readStringArg(raw, "title")
	return sourceCitation{URL: url, Title: title}, true
}
