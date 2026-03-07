package exporter

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	pdfPageWidth    = 595
	pdfPageHeight   = 842
	pdfMarginX      = 40
	pdfMarginTop    = 800
	pdfLineHeight   = 18
	pdfLinesPerPage = 40
)

// BuildSimplePDF は日本語を含むテキスト行を複数ページ PDF に整形する。
func BuildSimplePDF(lines []string) ([]byte, error) {
	pages := paginate(lines, pdfLinesPerPage)

	var objects []string
	fontObjectID := 3
	descendantFontObjectID := 4
	objects = append(objects, "<< /Type /Catalog /Pages 2 0 R >>")
	objects = append(objects, buildPagesObject(len(pages), 5))
	objects = append(objects, fmt.Sprintf(
		"<< /Type /Font /Subtype /Type0 /BaseFont /HeiseiKakuGo-W5 /Encoding /UniJIS-UCS2-H /DescendantFonts [%d 0 R] >>",
		descendantFontObjectID,
	))
	objects = append(objects, "<< /Type /Font /Subtype /CIDFontType0 /BaseFont /HeiseiKakuGo-W5 /CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 2 >> >>")

	pageObjectIDs := make([]int, 0, len(pages))
	contentObjectIDs := make([]int, 0, len(pages))
	nextObjectID := 5
	for range pages {
		pageObjectIDs = append(pageObjectIDs, nextObjectID)
		contentObjectIDs = append(contentObjectIDs, nextObjectID+1)
		nextObjectID += 2
	}

	for index, pageLines := range pages {
		pageObjectID := pageObjectIDs[index]
		contentObjectID := contentObjectIDs[index]
		objects = append(objects, buildPageObject(contentObjectID, fontObjectID))
		objects = append(objects, buildContentObject(pageLines))
		_ = pageObjectID
	}

	return assemblePDF(objects)
}

func paginate(lines []string, perPage int) [][]string {
	expanded := make([]string, 0, len(lines))
	for _, line := range lines {
		expanded = append(expanded, wrapPDFLine(line, 42)...)
	}
	if len(expanded) == 0 {
		expanded = []string{""}
	}

	pages := make([][]string, 0, (len(expanded)+perPage-1)/perPage)
	for start := 0; start < len(expanded); start += perPage {
		end := start + perPage
		if end > len(expanded) {
			end = len(expanded)
		}
		pages = append(pages, expanded[start:end])
	}
	return pages
}

func wrapPDFLine(line string, limit int) []string {
	normalized := strings.ReplaceAll(line, "\r\n", "\n")
	segments := strings.Split(normalized, "\n")
	wrapped := make([]string, 0, len(segments))
	for _, segment := range segments {
		runes := []rune(segment)
		if len(runes) == 0 {
			wrapped = append(wrapped, "")
			continue
		}
		for len(runes) > limit {
			wrapped = append(wrapped, string(runes[:limit]))
			runes = runes[limit:]
		}
		wrapped = append(wrapped, string(runes))
	}
	return wrapped
}

func buildPagesObject(pageCount int, firstPageObjectID int) string {
	kids := make([]string, 0, pageCount)
	for index := 0; index < pageCount; index++ {
		kids = append(kids, fmt.Sprintf("%d 0 R", firstPageObjectID+(index*2)))
	}
	return fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", pageCount, strings.Join(kids, " "))
}

func buildPageObject(contentObjectID int, fontObjectID int) string {
	return fmt.Sprintf(
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>",
		pdfPageWidth,
		pdfPageHeight,
		fontObjectID,
		contentObjectID,
	)
}

func buildContentObject(lines []string) string {
	var content bytes.Buffer
	y := pdfMarginTop
	for _, line := range lines {
		content.WriteString("BT\n")
		content.WriteString("/F1 12 Tf\n")
		content.WriteString(fmt.Sprintf("1 0 0 1 %d %d Tm\n", pdfMarginX, y))
		content.WriteString(fmt.Sprintf("<%s> Tj\n", utf16Hex(line)))
		content.WriteString("ET\n")
		y -= pdfLineHeight
	}

	stream := content.String()
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)
}

func utf16Hex(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return "FEFF"
	}

	var builder strings.Builder
	builder.WriteString("FEFF")
	for _, r := range runes {
		builder.WriteString(fmt.Sprintf("%04X", r))
	}
	return builder.String()
}

func assemblePDF(objects []string) ([]byte, error) {
	var buffer bytes.Buffer
	buffer.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)

	for index, object := range objects {
		offsets = append(offsets, buffer.Len())
		buffer.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", index+1, object))
	}

	xrefStart := buffer.Len()
	buffer.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))
	buffer.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		buffer.WriteString(fmt.Sprintf("%010d 00000 n \n", offset))
	}
	buffer.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF",
		len(objects)+1,
		xrefStart,
	))

	return buffer.Bytes(), nil
}
