package index

import (
	"encoding/xml"
	"errors"
	"io"
	"log"
	"strconv"
	"strings"
)

// processMiniOcr retrieves individual Alto files from DSpace, converts Alto to MiniOcr and adds to solr index.
func processMiniOcr(uuid string, annotationsMap map[string]string, altoFiles []string,
	manifestId string, settings Configuration) error {
	for i := 0; i < len(altoFiles); i++ {
		if len(altoFiles[i]) > 0 {
			alto, err := getAltoXml(annotationsMap[altoFiles[i]])
			if err != nil {
				return err
			}
			if len(alto) != 0 {
				var result, err = convert(&alto, i)
				if err != nil {
					return err
				} else {
					if settings.IndexType == "full" {
						var err = postToSolr(uuid, altoFiles[i], result, manifestId, settings)
						if err != nil {
							return errors.New("solr indexing failed: " + err.Error())
						}
					} else {
						var err = postToSolrLazyLoad(uuid, altoFiles[i], result, manifestId, settings)
						if err != nil {
							return errors.New("solr indexing failed: " + err.Error())
						}
					}
				}
			}
		}
	}
	return nil
}

// convert creates miniOcr output from the ALTO input.
func convert(alto *string, position int) (*string, error) {
	alto = convertToAscii(alto)
	reader := strings.NewReader(*alto)
	decoder := xml.NewDecoder(reader)

	ocr := &OcrEl{}

	pageElements := make([]P, 0)
	textBlockElements := make([]B, 0)
	lineElements := make([]L, 0)
	wordElements := make([]W, 0)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("error getting token: %t\n", err)
			return nil, err
			break
		}

		switch t := token.(type) {

		case xml.StartElement:
			if t.Name.Local == "Page" {
				textBlockElements = nil
				pageId := "Page." + strconv.Itoa(position)
				page := &P{Id: pageId}
				pageElements = append(pageElements, *page)
				ocr.Pages = pageElements
				continue
			}
			if t.Name.Local == "ComposedBlock" {
				continue
			}
			if t.Name.Local == "TextBlock" {
				lineElements = nil
				block := &B{}
				lastPage := &ocr.Pages[len(ocr.Pages)-1]
				textBlockElements = append(textBlockElements, *block)
				lastPage.Blocks = textBlockElements
				continue
			}
			if t.Name.Local == "TextLine" {
				wordElements = nil
				lineBlock := &L{}
				lastPage := &ocr.Pages[len(ocr.Pages)-1]
				if len(textBlockElements) > 0 {
					lastBlock := &lastPage.Blocks[len(textBlockElements)-1]
					lineElements = append(lineElements, *lineBlock)
					lastBlock.Lines = lineElements
				}
				continue
			}
			if t.Name.Local == "String" {
				content := t.Attr[0]
				height := t.Attr[1]
				width := t.Attr[2]
				vpos := t.Attr[3]
				hpos := t.Attr[4]
				coordinates := height.Value + " " + width.Value + " " + vpos.Value + " " + hpos.Value
				wordElement := W{CoorinateAttr: coordinates, Content: content.Value + " "}
				lastPage := &ocr.Pages[len(ocr.Pages)-1]
				lastBlock := &lastPage.Blocks[len(textBlockElements)-1]
				currentLine := &lastBlock.Lines[len(lineElements)-1]
				wordElements = append(wordElements, wordElement)
				currentLine.Words = wordElements
				continue
			}

		}

	}
	marshalledXml, err := xml.Marshal(ocr)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	out := string(marshalledXml)
	// Use single quotes in XML so we submit as json
	out = strings.ReplaceAll(out, "\"", "'")
	result := convertToAscii(&out)
	return result, nil
}