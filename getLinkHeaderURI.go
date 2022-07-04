package main

import (
	"fmt"
	"regexp"
)

/*
 * Returns the URI in a link header with the specified "rel" attribute
 * or an empty string when the "rel" attribute is not found. For example:
 * link = `<https://example.com>; rel="test"`
 * rel = `test`
 * returns `https://example.com`
 */
func getLinkHeaderURI(link string, rel string) string {
	reStr := fmt.Sprintf(`<([^,]*)>; rel="%v"`, rel)
	re := regexp.MustCompile(reStr)
	match := re.FindStringSubmatch(link)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}
