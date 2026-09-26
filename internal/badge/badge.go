// Package badge renders a report's score and grade as static SVG badges for
// a README. It only presents the report; it computes nothing about quality.
package badge

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"

	"github.com/iv-one/goquality/internal/check"
)

// Label is the left-hand text of every badge.
const Label = "Go Quality"

// Score renders "Go Quality | 92/100", rounded to the nearest point as the
// report rounds it.
func Score(rep check.Report) []byte {
	return SVG(Label, fmt.Sprintf("%d/100", int(math.Round(rep.Score))), Color(rep.Grade))
}

// Grade renders "Go Quality | A+".
func Grade(rep check.Report) []byte {
	return SVG(Label, string(rep.Grade), Color(rep.Grade))
}

// Color is the badge color for a grade, from green to red. The value on the
// badge carries the meaning; color only supplements it.
func Color(g check.Grade) string {
	switch g {
	case check.GradeAPlus:
		return "#44cc11"
	case check.GradeA:
		return "#97ca00"
	case check.GradeB:
		return "#a4a61d"
	case check.GradeC:
		return "#dfb317"
	case check.GradeD:
		return "#fe7d37"
	}
	return "#e05d44"
}

// SVG renders a flat two-part badge. The output is self-contained (no
// scripts, styles or remote assets) and deterministic for the same input.
func SVG(label, value, color string) []byte {
	lw, vw := textWidth(label)+10, textWidth(value)+10
	w := lw + vw
	title := escape(label + ": " + value)
	var b bytes.Buffer
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s">`, w, title)
	fmt.Fprintf(&b, `<title>%s</title>`, title)
	b.WriteString(`<linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>`)
	fmt.Fprintf(&b, `<clipPath id="r"><rect width="%d" height="20" rx="3" fill="#fff"/></clipPath>`, w)
	fmt.Fprintf(&b, `<g clip-path="url(#r)"><rect width="%d" height="20" fill="#555"/><rect x="%d" width="%d" height="20" fill="%s"/><rect width="%d" height="20" fill="url(#s)"/></g>`,
		lw, lw, vw, escape(color), w)
	b.WriteString(`<g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">`)
	text(&b, float64(lw)/2, lw-10, label)
	text(&b, float64(lw)+float64(vw)/2, vw-10, value)
	b.WriteString("</g></svg>\n")
	return b.Bytes()
}

// text writes s centered at x with a drop shadow. textLength pins the width
// to the estimate, so the badge looks the same whatever font is available.
func text(b *bytes.Buffer, x float64, length int, s string) {
	s = escape(s)
	fmt.Fprintf(b, `<text x="%.1f" y="15" fill="#010101" fill-opacity=".3" textLength="%d">%s</text>`, x, length, s)
	fmt.Fprintf(b, `<text x="%.1f" y="14" textLength="%d">%s</text>`, x, length, s)
}

// widths are advance widths of Verdana at 11px for the characters badges
// use; others fall back to an average.
var widths = map[rune]float64{
	' ': 3.87, '/': 4.93, '+': 9.21, '-': 4.99, '.': 3.87, '%': 12.06,
	'0': 7, '1': 7, '2': 7, '3': 7, '4': 7, '5': 7, '6': 7, '7': 7, '8': 7, '9': 7,
	'A': 7.52, 'B': 7.54, 'C': 7.68, 'D': 8.48, 'E': 6.96, 'F': 6.32, 'G': 8.38, 'Q': 8.66,
	'a': 6.66, 'i': 3.08, 'l': 3.08, 'o': 6.72, 't': 4.26, 'u': 7.12, 'y': 6.51,
}

func textWidth(s string) int {
	var w float64
	for _, r := range s {
		if cw, ok := widths[r]; ok {
			w += cw
		} else {
			w += 7
		}
	}
	return int(math.Ceil(w))
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s)) // writes to a bytes.Buffer, which cannot fail
	return b.String()
}
