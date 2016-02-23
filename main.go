package main

/////////////////////////////////////////////////////////////////
//Code generated by chidley https://github.com/gnewton/chidley //
/////////////////////////////////////////////////////////////////

import (
	"encoding/xml"
	"flag"
	"io"
	"io/ioutil"
	//_ "net/http/pprof"
	"strings"
	//"github.com/davecheney/profile"
	"github.com/jinzhu/gorm"
	"net/http"

	"log"
	//	"net/http"
	"os"
	"strconv"
	//"strings"
	"math"
	"time"
)

var TransactionSize = 5000
var chunkSize = 1000
var CloseOpenSize int64 = 99950000
var chunkChannelSize = 3
var dbFileName = "./pubmed_sqlite.db"
var sqliteLogFlag = false
var LoadNRecordsPerFile int64 = math.MaxInt64
var recordPerFileCounter int64 = 0
var doNotWriteToDbFlag = false

const CommentsCorrections_RefType = "Cites"
const PUBMED_ARTICLE = "PubmedArticle"

var out int = -1
var JournalIdCounter int64 = 0
var counters map[string]*int
var closeOpenCount int64 = 0

func init() {
	//defer profile.Start(profile.CPUProfile).Stop()
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.BoolVar(&sqliteLogFlag, "L", sqliteLogFlag, "Turn on sqlite logging")
	flag.StringVar(&dbFileName, "f", dbFileName, "SQLite output filename")

	flag.IntVar(&TransactionSize, "t", TransactionSize, "Size of transactions")
	flag.IntVar(&chunkSize, "C", chunkSize, "Size of chunks")
	flag.Int64Var(&CloseOpenSize, "z", CloseOpenSize, "Num of records before sqlite connection is closed then reopened")
	flag.Int64Var(&LoadNRecordsPerFile, "N", LoadNRecordsPerFile, "Load only N records from each file")
	flag.BoolVar(&sqliteLogFlag, "V", sqliteLogFlag, "Turn on sqlite logging")

	flag.BoolVar(&doNotWriteToDbFlag, "X", doNotWriteToDbFlag, "Do not write to db. Rolls back transaction. For debugging")

	flag.Parse()

	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	logInit(ioutil.Discard, os.Stdout, os.Stdout, os.Stderr)
}

func main() {
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	//defer profile.Start(profile.CPUProfile).Stop()

	db, err := dbInit()
	if err != nil {
		Error.Fatal(err)
		return
	}
	defer func() {
		err = db.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	articleChannel := make(chan []*Article, chunkChannelSize)

	done := make(chan bool)

	go articleAdder(articleChannel, done, db, TransactionSize)
	var count int64 = 0
	chunkCount := 0
	arrayIndex := 0

	var articleArray []*Article

	for i, filename := range flag.Args() {
		log.Println(i, " -- Input file: "+filename)
	}

	// Loop through files
	for i, filename := range flag.Args() {

		log.Println("Opening: "+filename, " ", i+1, " of ", len(flag.Args()))
		//log.Println(strconv.Itoa(i) + " of " + strconv.Itoa(len(flag.Args)-1))
		reader, _, err := genericReader(filename)

		if err != nil {
			log.Fatal(err)
			return
		}
		arrayIndex = 0
		articleArray = make([]*Article, chunkSize)

		decoder := xml.NewDecoder(reader)
		counters = make(map[string]*int)

		// Loop through XML
		for {
			if recordPerFileCounter == LoadNRecordsPerFile {
				log.Println("break file load. LoadNRecordsPerFile", count, LoadNRecordsPerFile)
				recordPerFileCounter = 0
				break
			}
			token, _ := decoder.Token()
			if token == nil {
				break
			}
			switch se := token.(type) {
			case xml.StartElement:
				if se.Name.Local == PUBMED_ARTICLE && se.Name.Space == "" {
					if count%10000 == 0 && count != 0 {
						log.Println("------------")
						log.Printf("count=%d\n", count)
						log.Printf("arrayIndex=%d\n", arrayIndex)
						log.Println("------------")
					}

					count = count + 1
					recordPerFileCounter = recordPerFileCounter + 1
					var pubmedArticle ChiPubmedArticle
					decoder.DecodeElement(&pubmedArticle, &se)
					article := pubmedArticleToDbArticle(&pubmedArticle)
					if article == nil {
						log.Println("-----------------nil")
						continue
					}
					articleArray[arrayIndex] = article
					arrayIndex = arrayIndex + 1
					if arrayIndex >= chunkSize {
						//log.Printf("Sending chunk %d", chunkCount)
						chunkCount = chunkCount + 1
						//pubmedArticleChannel <- &pubmedArticle
						//log.Printf("%v\n", articleArray)
						articleChannel <- articleArray
						log.Println("Sent")
						articleArray = make([]*Article, chunkSize)
						arrayIndex = 0
					}
				}
			}

		}
		if arrayIndex > 0 && arrayIndex < chunkSize {
			articleChannel <- articleArray
			chunkCount = chunkCount + 1
		}
	}

	close(articleChannel)
	_ = <-done

}

func pubmedArticleToDbArticle(p *ChiPubmedArticle) *Article {
	medlineCitation := p.ChiMedlineCitation
	pArticle := medlineCitation.ChiArticle
	if pArticle == nil {
		log.Println("nil-----------")
		return nil
	}
	var err error
	dbArticle := new(Article)
	dbArticle.ID, err = strconv.ParseInt(p.ChiMedlineCitation.ChiPMID.Text, 10, 64)
	if err != nil {
		log.Println(err)
	}
	dbArticle.Abstract = ""
	//if pArticle !=pArticle.ChiAbstract != nil && pArticle.ChiAbstract.ChiAbstractText != nil {
	if pArticle.ChiAbstract != nil && pArticle.ChiAbstract.ChiAbstractText != nil {
		for i, _ := range pArticle.ChiAbstract.ChiAbstractText {
			dbArticle.Abstract = dbArticle.Abstract + " " + pArticle.ChiAbstract.ChiAbstractText[i].Text
		}
	}

	dbArticle.Title = pArticle.ChiArticleTitle.Text

	if pArticle.ChiJournal != nil {
		if pArticle.ChiJournal.ChiJournalIssue != nil {
			if pArticle.ChiJournal.ChiJournalIssue.ChiPubDate != nil {
				if pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiYear != nil {
					dbArticle.Year, err = strconv.Atoi(pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiYear.Text)

					if err != nil {
						log.Println(err)
					}

				} else {
					if pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMedlineDate == nil || pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMedlineDate.Text == "" {
						log.Println("MedlineDate is nil? ", pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMedlineDate)
					} else {
						dbArticle.Year = medlineDate2Year(pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMedlineDate.Text)
					}
				}
				if pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMonth != nil {
					dbArticle.Month = pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMonth.Text
				}
				if pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiDay != nil {
					dbArticle.Day, err = strconv.Atoi(pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiDay.Text)
					if err != nil {
						log.Println(err)
					}
				}
			} else {
				log.Println("ChiJournal.ChiJournalIssue.ChiPubDate=nil pmid=", dbArticle.ID)
			}
			if dbArticle.Year < 1000 {
				log.Println("*******************************************")
				log.Println("Year=Error ", dbArticle.ID)
				log.Println(dbArticle.Year)
				log.Printf("%+v\n", pArticle.ChiJournal.ChiJournalIssue)

				log.Printf("%+v\n", pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiYear)
				if pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMedlineDate != nil {
					log.Printf("%+v\n", pArticle.ChiJournal.ChiJournalIssue.ChiPubDate.ChiMedlineDate.Text)
				}
				log.Println("*******************************************")
			}
		}
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
		var err error
		for _, commentsCorrection := range medlineCitation.ChiCommentsCorrectionsList.ChiCommentsCorrections {
			if commentsCorrection.Attr_RefType == CommentsCorrections_RefType {
				citation := new(Citation)
				citation.Pmid, err = strconv.ParseInt(commentsCorrection.ChiPMID.Text, 10, 64)
				if err != nil {
					log.Println(err)
				}
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

	// if medlineCitation.ChiMeshHeadingList != nil {
	// 	dbArticle.MeshTerms = make([]MeshTerm, len(medlineCitation.ChiMeshHeadingList.ChiMeshHeading))
	// 	for i, mesh := range medlineCitation.ChiMeshHeadingList.ChiMeshHeading {
	// 		dbMesh := new(MeshTerm)
	// 		dbMesh.Descriptor = mesh.ChiDescriptorName.Text
	// 		//dbMesh.Qualifier = mesh.ChiQualifierName.Text
	// 		dbArticle.MeshTerms[i] = *dbMesh
	// 	}
	// }

	if pArticle.ChiJournal != nil {
		//journal := Journal{}
		//db.First(&journal, 10)
		//db.First(&user, 10)
		//db.Where("name = ?", "hello world").First(&User{}).Error == gorm.RecordNotFound
		//fmt.Println(pArticle.ChiJournal.ChiTitle.Text)
		journal := Journal{
			Title: pArticle.ChiJournal.ChiTitle.Text,
		}
		//journal := new(Journal)
		//journal.Id = JournalIdCounter
		//journal.Title = pArticle.ChiJournal.ChiTitle.Text
		if pArticle.ChiJournal.ChiISSN != nil {
			journal.Issn = pArticle.ChiJournal.ChiISSN.Text
		}
		dbArticle.Journal = journal
		//dbArticle.journal_id.Int64 = journal.Id
		//dbArticle.journal_id.Valid = true
		//JournalIdCounter = JournalIdCounter + 1

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
			if author.ChiAffiliation != nil {
				dbAuthor.Affiliation = author.ChiAffiliation.Text
			}
			dbArticle.Authors[i] = *dbAuthor
		}
	}

	return dbArticle
}

func articleAdder(articleChannel chan []*Article, done chan bool, db *gorm.DB, commitSize int) {
	log.Println("Start articleAdder")
	tx := db.Begin()
	t0 := time.Now()
	var totalCount int64 = 0
	counter := 0
	chunkCount := 0
	for articleArray := range articleChannel {
		log.Println("-- Consuming chunk ", chunkCount)

		log.Printf("articleAdder counter=%d", counter)
		log.Printf("TOTAL counter=%d", totalCount)

		log.Println(commitSize)
		if doNotWriteToDbFlag {
			counter = counter + len(articleArray)
			totalCount = totalCount + int64(len(articleArray))
			continue
		}

		tmp := articleArray
		for i := 0; i < len(tmp); i++ {
			article := tmp[i]
			if article == nil {
				//log.Println(i, " ******** Article is nil")
				continue
			}

			counter = counter + 1
			totalCount = totalCount + 1
			closeOpenCount = closeOpenCount + 1
			if counter == commitSize {
				tc0 := time.Now()
				tx.Commit()
				t1 := time.Now()
				log.Printf("The commit took %v to run.\n", t1.Sub(tc0))
				log.Printf("The call took %v to run.\n", t1.Sub(t0))
				t0 = time.Now()
				counter = 0

				if closeOpenCount >= CloseOpenSize {
					log.Println("CLOSEOPEN $$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$ ", closeOpenCount)
					var err error
					db, err = dbCloseOpen(db)
					if err != nil {
						log.Fatal(err)
					}
					closeOpenCount = 0
				}

				tx = db.Begin()
			}
			if err := tx.Create(article).Error; err != nil {
				tx.Rollback()
				log.Println("\\\\\\\\\\\\\\\\")
				log.Println("[", err, "]")
				log.Printf("PMID=%d", article.ID)
				//if !strings.HasSuffix(err.Error(), "PRIMARY KEY must be unique") {
				//continue
				//}
				//log.Println("Returning from articleAdder")
				//log.Fatal(" Fatal\\\\\\\\\\\\\\\\")
				//return
				tx = db.Begin()
			}

		}
		log.Println("-- END chunk ", chunkCount)
	}
	if !doNotWriteToDbFlag {
		tx.Commit()
		makeIndexes(db)
	}
	db.Close()
	done <- true
}

// From: http://www.goinggo.net/2013/11/using-log-package-in-go.html
var (
	Trace   *log.Logger
	Info    *log.Logger
	Warning *log.Logger
	Error   *log.Logger
)

func logInit(
	traceHandle io.Writer,
	infoHandle io.Writer,
	warningHandle io.Writer,
	errorHandle io.Writer) {

	Trace = log.New(traceHandle,
		"TRACE: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Info = log.New(infoHandle,
		"INFO: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Warning = log.New(warningHandle,
		"WARNING: ",
		log.Ldate|log.Ltime|log.Lshortfile)

	Error = log.New(errorHandle,
		"ERROR: ",
		log.Ldate|log.Ltime|log.Lshortfile)
}

func medlineDate2Year(md string) int {
	// case <MedlineDate>1952 Mar-Apr</MedlineDate>
	var year int
	var err error
	// case 2000-2001
	if string(md[4]) == string('-') {
		yearStrings := strings.Split(md, "-")
		//case 1999-00
		if len(yearStrings[1]) != 4 {
			year, err = strconv.Atoi(yearStrings[0])
		} else {
			year, err = strconv.Atoi(yearStrings[1])
		}
		if err != nil {
			log.Println("error!! ", err)
		}
	} else {
		// case 1999 June 6
		yearString := strings.TrimSpace(strings.Split(md, " ")[0])
		yearString = yearString[0:4]
		//year, err = strconv.Atoi(strings.TrimSpace(strings.Split(md, " ")[0]))
		year, err = strconv.Atoi(yearString)
		if err != nil {
			log.Println("error!! ", err)
		}
	}
	if year == 0 {
		log.Println("medlineDate2Year ", year, md, " [", strings.TrimSpace(string(md[4])), "]")
	}
	return year

}
