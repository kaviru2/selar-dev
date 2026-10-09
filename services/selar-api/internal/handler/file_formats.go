package handler

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

func fileFormat(filename string) (kind, mime string) {
	switch strings.ToLower(path.Ext(filename)) {
	case ".pdf":
		return "pdf", pdfContentType
	case ".md", ".markdown":
		return "markdown", "text/markdown"
	case ".txt":
		return "txt", "text/plain"
	case ".docx":
		return "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "", ""
	}
}

func validateFile(data []byte, kind string) error {
	if len(data) == 0 || int64(len(data)) > maxPDFSize {
		return fmt.Errorf("file is empty or too large")
	}
	if kind == "pdf" {
		if !bytes.HasPrefix(data, pdfMagic) {
			return fmt.Errorf("file is not a valid PDF")
		}
		return nil
	}
	if kind == "markdown" || kind == "txt" {
		if len(data) > 10<<20 || !utf8.Valid(data) || bytes.ContainsRune(data, 0) || strings.TrimSpace(string(data)) == "" {
			return fmt.Errorf("file must contain nonempty UTF-8 text (maximum 10 MB)")
		}
		return nil
	}
	if kind == "docx" {
		return validateDOCX(data)
	}
	return fmt.Errorf("unsupported file format")
}

func validateDOCX(data []byte) error {
	invalid := fmt.Errorf("invalid or unsupported DOCX: require standard OOXML without macros, tracked changes, comments or footnotes (10 MB maximum)")
	if len(data) > 10<<20 {
		return invalid
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) > 1000 {
		return invalid
	}
	seen := map[string]bool{}
	parts := map[string][]byte{}
	var total uint64
	for _, file := range archive.File {
		name := file.Name
		lower := strings.ToLower(name)
		if seen[name] || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != strings.TrimSuffix(name, "/") || strings.HasPrefix(name, "../") || file.Flags&1 != 0 {
			return invalid
		}
		seen[name] = true
		for _, marker := range []string{"vba", "macros", "comments", "footnotes", "endnotes"} {
			if strings.Contains(lower, marker) {
				return invalid
			}
		}
		total += file.UncompressedSize64
		if file.UncompressedSize64 > 8<<20 || total > 32<<20 || file.UncompressedSize64 > max(uint64(1), file.CompressedSize64)*100 {
			return invalid
		}
		reader, err := file.Open()
		if err != nil {
			return invalid
		}
		value, err := io.ReadAll(io.LimitReader(reader, (8<<20)+1))
		reader.Close()
		if err != nil || uint64(len(value)) != file.UncompressedSize64 {
			return invalid
		}
		if name == "[Content_Types].xml" || name == "_rels/.rels" || name == "word/document.xml" {
			parts[name] = value
		}
	}
	var types struct {
		Items []struct {
			Name string `xml:"PartName,attr"`
			Type string `xml:"ContentType,attr"`
		} `xml:"Override"`
	}
	if xml.Unmarshal(parts["[Content_Types].xml"], &types) != nil {
		return invalid
	}
	found := false
	for _, item := range types.Items {
		if item.Name == "/word/document.xml" && item.Type == "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml" {
			found = true
		}
	}
	if !found {
		return invalid
	}
	var rels struct {
		Items []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(parts["_rels/.rels"], &rels) != nil {
		return invalid
	}
	found = false
	for _, item := range rels.Items {
		if strings.HasSuffix(item.Type, "/officeDocument") && item.Target == "word/document.xml" && item.Mode != "External" {
			found = true
		}
	}
	if !found {
		return invalid
	}
	decoder := xml.NewDecoder(bytes.NewReader(parts["word/document.xml"]))
	found = false
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return invalid
		}
		if _, ok := token.(xml.Directive); ok {
			return invalid
		}
		if start, ok := token.(xml.StartElement); ok {
			depth++
			if depth > 128 {
				return invalid
			}
			if !found {
				if start.Name.Local != "document" || start.Name.Space != "http://schemas.openxmlformats.org/wordprocessingml/2006/main" {
					return invalid
				}
				found = true
			}
			switch start.Name.Local {
			case "ins", "del", "moveFrom", "moveTo", "commentReference", "footnoteReference", "endnoteReference", "altChunk":
				return invalid
			}
		}
		if _, ok := token.(xml.EndElement); ok {
			depth--
		}
	}
	if !found {
		return invalid
	}
	return nil
}
