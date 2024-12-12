package fb2text

import (
	"archive/zip"
	"encoding/xml"
	"net/http"
	"os"
	"strings"

	xs "github.com/huandu/xstrings"
	"golang.org/x/net/html/charset"
)

/*
BookInfo is a short information about FB2 book. It supports few tags
only: book title, first and last author names, sequence, genre, and
text language (not the original book language)
*/
type BookInfo struct {
	Authors  []Author
	Title    string
	Sequence string
	Language string
	Genre    string
}

type Author struct {
	FirstName string
	LastName  string
}

/*
IsZipFile checks if the file is ZIP archive.
Returns true is the file is ZIP or GZIP archive and false otherwise
*/
func IsZipFile(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}

	defer file.Close()
	buff := make([]byte, 512)
	if _, err = file.Read(buff); err != nil {
	}

	fileType := http.DetectContentType(buff)
	switch fileType {
	case "application/x-gzip", "application/zip":
		return true
	default:
		return false
	}
}

func isInBookInfo(path []string) bool {
	if len(path) < 3 {
		return false
	}

	return path[0] == "FictionBook" &&
		path[1] == "description" &&
		path[2] == "title-info"
}

func isInBookContent(path []string) bool {
	if len(path) < 2 {
		return false
	}

	return path[0] == "FictionBook" &&
		path[1] == "body"
}

func isInside(path []string, sectionName string) bool {
	n := len(path) - 1
	if n < 0 {
		return false
	}

	for n >= 0 {
		if path[n] == sectionName {
			return true
		}
		if path[n] == "p" || path[n] == "emphasis" || path[n] == "text-author" || path[n] == "strong" {
			n--
		} else {
			break
		}
	}

	return false
}

/*
ParseBook converts FB2 file to a simple list of strings with some extra
information to display the text correctly. So, the parsed text is not for
immediate display. It should be preformatted before showing to a user.

fileName - path to file contains FB2 formatted text. It can be ZIP archive,

	the function automatically unpack zip files

parseBody - if parseBody is false the function stops right after it hits the

	first 'body' tag. By this time all book information is read. The parameter
	can be used for quick read of book properties without parsing the entire
	file

Returns information about book[see BookInfo structure] and (if parseBody equals

	true) the parsed FB2 text in internal format. Please read more about format
	below.

All tags are enclosed in double curly brackets, like "{{section}}"
Since terminal is not rich with GUI features, only few FB2 tags are added
to output text. Existing internal tags:
The following tags are always at the very beginning of the line:
{{section}} - defines section start. Default format adds extra empty line
{{title}} - defines title line. There can be several title lines in a row.

	Default format justify the title in the center of screen if title length is
	smaller than screen width. Otherwise it is displayed as regular paragraph

{{epi}} - defines ephigraph start. Default format takes all consecutive epigraph

	lines, calculates the maximal width and then format all epigraph lines to make
	them right justified in such way that the longest string ends at the right
	edge of the screen

{{epiauth}} - defines author of the epigraph text start. Default format treats

	this tag as if it is {{epi}} one.

The following tags can be in any place of the string, that is why thay have
starting and ending markers:
{{emon}} and {{emoff}} - defines emphasized text started. Default format skips
these tags and does nothing. In original FB2 two tags are mapped to {{emon}}:
<strong> and <emphasis>

If a parsed string does not start with "{{" it means the string is regular
paragraph of text. Default format separates the section to lines not longer
than screen width. If a string is longer and do not have spaces then the string
just divided at screen width position. If option 'justify' is set then all
string of the paragraph(except the last one) are expanded with extra spaces to
make all string the same widthop
*/
func ParseBook(fileName string, opts ...FOption) (BookInfo, []string) {
	opt := option{}

	for _, fun := range opts {
		opt = fun(opt)
	}

	isZip := IsZipFile(fileName)

	lines := make([]string, 0)
	var binfo BookInfo
	tags := make([]string, 0, 10)

	var decoder *xml.Decoder

	if isZip {
		zp, err := zip.OpenReader(fileName)
		if err != nil {
			return binfo, lines
		}

		defer zp.Close()

		for _, f := range zp.File {
			if strings.HasSuffix(f.Name, ".fb2") {
				zipFb2, err := f.Open()
				if err != nil {
					return binfo, lines
				}
				decoder = xml.NewDecoder(zipFb2)
				defer zipFb2.Close()
				break
			}
		}
	} else {
		xmlFile, err := os.Open(fileName)
		if err != nil {
			return binfo, lines
		}
		defer xmlFile.Close()
		decoder = xml.NewDecoder(xmlFile)
	}

	if decoder == nil {
		return binfo, lines
	}

	decoder.CharsetReader = charset.NewReaderLabel

	var currLine string
	for {
		t, _ := decoder.Token()
		if t == nil {
			break
		}

		// Inspect the type of the token just read.
		switch se := t.(type) {
		case xml.StartElement:
			if !opt.parseBody && se.Name.Local == "body" {
				return binfo, lines
			}

			if se.Name.Local == "empty-line" && !opt.skipSystemLines {
				lines = append(lines, "")
				currLine = ""
			} else if se.Name.Local == "section" && !opt.skipSystemLines {
				lines = append(lines, "{{section}}")
				currLine = ""
			} else if (se.Name.Local == "emphasis" || se.Name.Local == "strong") && !opt.skipSystemLines {
				currLine += "{{emon}}"
			} else if se.Name.Local == "sequence" {
				for i := 0; i < len(se.Attr); i++ {
					if se.Attr[i].Name.Local == "name" {
						binfo.Sequence = se.Attr[i].Value
					}
				}
			} else {
				if se.Name.Local == "text-author" && isInside(tags, "epigraph") {
					currLine = "{{epiauth}}"
				} else if se.Name.Local == "p" {
					if isInside(tags, "epigraph") {
						currLine = "{{epi}}"
					} else if isInside(tags, "title") {
						currLine = "{{title}}"
					} else {
						currLine = ""
					}
				} else {
					currLine = ""
				}
			}
			tags = append(tags, se.Name.Local)
		case xml.EndElement:
			if tags[len(tags)-1] != se.Name.Local {
				panic("Invalid fb2")
			}
			tags = tags[:len(tags)-1]
			if isInBookInfo(tags) {
				if se.Name.Local == "genre" {
					binfo.Genre = currLine
				} else if se.Name.Local == "first-name" && isInside(tags, "author") {
					binfo.Authors = append(binfo.Authors, Author{FirstName: currLine})
				} else if se.Name.Local == "last-name" && isInside(tags, "author") {
					last := len(binfo.Authors) - 1
					author := binfo.Authors[last]
					author.LastName = currLine
					binfo.Authors[last] = author
				} else if se.Name.Local == "book-title" {
					binfo.Title = currLine
				} else if se.Name.Local == "lang" {
					binfo.Language = currLine
				}
			} else if isInBookContent(tags) {
				if se.Name.Local == "body" {
					return binfo, lines
				} else if se.Name.Local == "emphasis" || se.Name.Local == "strong" {
					currLine += "{{emoff}}"
				} else {
					if currLine != "" {
						lines = append(lines, currLine)
					}
					currLine = ""
				}
			} else {
				currLine = ""
			}
		case xml.CharData:
			ss := string(se)
			newLines := xs.Count(ss, "\n\r ")
			if newLines != len(ss) {
				ss = xs.Squeeze(xs.Translate(ss, "\n\r", "  "), " ")
				currLine += ss
			}
		}
	}

	return binfo, lines
}
