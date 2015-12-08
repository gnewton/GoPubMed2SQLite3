package main

/////////////////////////////////////////////////////////////////
//Code generated by chidley https://github.com/gnewton/chidley //
/////////////////////////////////////////////////////////////////

import (
	"encoding/xml"
	//"flag"
	"fmt"
	//"github.com/davecheney/profile"
	"github.com/jinzhu/gorm"
	"log"
	//	"net/http"
	"os"
	"strconv"
	"time"
)

var filename = "/home/gnewton/newtong/work/pubmedDownloadXmlById/aa/pubmed_xml_26419650"

const TransactionSize = 100000
const ArticleBufferSize = 200000

const CommentsCorrections_RefType = "Cites"

func init() {

}

var out int = -1

var counters map[string]*int

const PUBMED_ARTICLE = "PubmedArticle"

func main() {

	//defer profile.Start(profile.CPUProfile).Stop()

	db, err := dbInit()
	if err != nil {
		log.Fatal(err)
		return
	}

	pubmedArticleChannel := make(chan *ChiPubmedArticle, ArticleBufferSize)

	done := make(chan bool)

	go articleAdder(pubmedArticleChannel, done, db, TransactionSize)
	count := 0

	for i, filename := range os.Args {
		if i == 0 {
			continue
		}
		log.Println("Opening: " + filename)
		log.Println(strconv.Itoa(i) + " of " + strconv.Itoa(len(os.Args)))
		reader, _, err := genericReader(filename)
		if err != nil {
			log.Fatal(err)
			return
		}

		decoder := xml.NewDecoder(reader)
		counters = make(map[string]*int)

		for {

			token, _ := decoder.Token()
			if token == nil {
				break
			}
			switch se := token.(type) {
			case xml.StartElement:
				//if se.Name.Local == PUBMED_ARTICLE || se.Name.Local == "PubmedBookArticle" {
				//
				//}
				if se.Name.Local == PUBMED_ARTICLE && se.Name.Space == "" {
					if count%10000 == 0 && count != 0 {
						fmt.Println("------------")
						fmt.Println(count)
						fmt.Println("------------")
					}

					count = count + 1
					var pubmedArticle ChiPubmedArticle
					decoder.DecodeElement(&pubmedArticle, &se)
					pubmedArticleChannel <- &pubmedArticle
				}
			}
		}
	}
	close(pubmedArticleChannel)
	_ = <-done
}

func pubmedArticleToDbArticle(p *ChiPubmedArticle) *Article {
	medlineCitation := p.ChiMedlineCitation
	pArticle := medlineCitation.ChiArticle
	dbArticle := new(Article)
	dbArticle.Id, _ = strconv.ParseInt(p.ChiMedlineCitation.ChiPMID.Text, 10, 64)
	dbArticle.Abstract = ""
	if pArticle.ChiAbstract != nil && pArticle.ChiAbstract.ChiAbstractText != nil {
		for i, _ := range pArticle.ChiAbstract.ChiAbstractText {
			dbArticle.Abstract = dbArticle.Abstract + " " + pArticle.ChiAbstract.ChiAbstractText[i].Text
		}
	}

	dbArticle.Title = pArticle.ChiArticleTitle.Text
	if pArticle.ChiArticleDate != nil {
		dbArticle.Year, _ = strconv.Atoi(pArticle.ChiArticleDate.ChiYear.Text)
		dbArticle.Month = pArticle.ChiArticleDate.ChiMonth.Text
		dbArticle.Day, _ = strconv.Atoi(pArticle.ChiArticleDate.ChiDay.Text)
	}

	if medlineCitation.ChiCommentsCorrectionsList != nil {
		actualCitationCount := 0
		for _, commentsCorrection := range medlineCitation.ChiCommentsCorrectionsList.ChiCommentsCorrections {
			if commentsCorrection.Attr_RefType == CommentsCorrections_RefType {
				actualCitationCount = actualCitationCount + 1
			}
		}

		dbArticle.Citations = make([]Citation, actualCitationCount)
		counter := 0
		for _, commentsCorrection := range medlineCitation.ChiCommentsCorrectionsList.ChiCommentsCorrections {
			if commentsCorrection.Attr_RefType == CommentsCorrections_RefType {
				citation := new(Citation)
				citation.Pmid, _ = strconv.ParseInt(commentsCorrection.ChiPMID.Text, 10, 64)
				citation.RefSource = commentsCorrection.ChiRefSource.Text
				dbArticle.Citations[counter] = *citation
				counter = counter + 1
			}
		}

	}

	if medlineCitation.ChiChemicalList != nil {
		dbArticle.Chemicals = make([]Chemical, len(medlineCitation.ChiChemicalList.ChiChemical))
		for i, chemical := range medlineCitation.ChiChemicalList.ChiChemical {
			dbChemical := new(Chemical)
			dbChemical.Name = chemical.ChiNameOfSubstance.Text
			dbChemical.Registry = chemical.ChiRegistryNumber.Text
			dbArticle.Chemicals[i] = *dbChemical
		}

	}

	if medlineCitation.ChiMeshHeadingList != nil {
		dbArticle.MeshTerms = make([]MeshTerm, len(medlineCitation.ChiMeshHeadingList.ChiMeshHeading))
		for i, mesh := range medlineCitation.ChiMeshHeadingList.ChiMeshHeading {
			dbMesh := new(MeshTerm)
			dbMesh.Descriptor = mesh.ChiDescriptorName.Text
			//dbMesh.Qualifier = mesh.ChiQualifierName.Text
			dbArticle.MeshTerms[i] = *dbMesh
		}
	}

	if pArticle.ChiJournal != nil {
		journal := new(Journal)
		journal.Title = pArticle.ChiJournal.ChiTitle.Text
		if pArticle.ChiJournal.ChiISSN != nil {
			journal.Issn = pArticle.ChiJournal.ChiISSN.Text
		}
		dbArticle.Journal = *journal
	}

	if pArticle.ChiAuthorList != nil {
		dbArticle.Authors = make([]Author, len(pArticle.ChiAuthorList.ChiAuthor))
		for i, author := range pArticle.ChiAuthorList.ChiAuthor {
			dbAuthor := new(Author)
			if author.ChiIdentifier != nil {
				//dbAuthor.Id = author.ChiIdentifier.Text
			}
			if author.ChiLastName != nil {
				dbAuthor.LastName = author.ChiLastName.Text
			}
			if author.ChiForeName != nil {
				dbAuthor.FirstName = author.ChiForeName.Text
			}
			dbArticle.Authors[i] = *dbAuthor
		}
	}

	return dbArticle
}

func articleAdder(pubmedArticleChannel chan *ChiPubmedArticle, done chan bool, db *gorm.DB, commitSize int) {

	//commitChannel := make(chan *gorm.DB, 10)
	//doneCommitting := make(chan bool)

	//go committer(commitChannel, doneCommitting, db)

	tx := db.Begin()
	t0 := time.Now()
	counter := 0
	for pubmedArticle := range pubmedArticleChannel {
		counter = counter + 1
		if counter%commitSize == 0 {
			//commitChannel <- tx
			fmt.Printf("++++++++++++ Starting commit: %v \n", t0)
			tx.Commit()
			t1 := time.Now()
			fmt.Printf("++++++++++++ The call took %v to run.\n", t1.Sub(t0))
			t0 = time.Now()
			tx = db.Begin()
		}
		dbArticle := pubmedArticleToDbArticle(pubmedArticle)
		if err := tx.Create(dbArticle).Error; err != nil {
			//tx.Rollback()
			log.Println("\\\\\\\\\\\\\\\\")
			//log.Fatal(err)
		}
	}
	//commitChannel <- tx
	//close(commitChannel)
	db.Close()
	//_ = <-doneCommitting
	done <- true
}

func committer(transactionChannel chan *gorm.DB, doneCommitting chan bool, db *gorm.DB) {
	for tx := range transactionChannel {
		t0 := time.Now()
		tx.Commit()
		t1 := time.Now()
		fmt.Printf("The call took %v to run.\n", t1.Sub(t0))
	}
	doneCommitting <- true
}
