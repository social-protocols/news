package main

import (
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"

	hn "github.com/peterhellberg/hn"
)

type FrontPageData struct {
	Stories []hn.Item
}

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

func main() {
	fmt.Println("In main")

	go runCrawler()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"

	}

	tmpl := template.Must(template.ParseFiles("templates/index.html.tmpl"))

	sampleStory := hn.Item{
		ID:          8863,
		By:          "dhouston",
		Parent:      8862,
		Kids:        []int{},
		Descendants: 71,
		Score:       111,
		Title:       "My YC app: Dropbox - Throw away your USB drive",
		URL:         "http://www.getdropbox.com/u/2/screencast.html",
		Timestamp:   1175714200,
	}
	sampleStories := []hn.Item{
		sampleStory,
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl.Execute(w, FrontPageData{sampleStories})
	})

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getNewStories() {

	//	Get the ID of the last story that has been submitted
	//
	//
	//	{
	//		resp, err := http.Get("https://hacker-news.firebaseio.com/v0/maxitem.json")
	//		if err != nil {
	//		   log.Fatalln(err)
	//		}
	//
	//	    i, err := strconv.Atoi(s)
	//	    if err != nil {
	//	        // ... handle error
	//			log.Fatal(err)
	//	    }
	//
	//	}
	//
	//	Get the highest ID you have in the databse
	//
	//		{
	//			for i := maxInDatabase+1; i <= maxStoryId; i++ {
	//				add story to database with:
	//					https: //hacker-news.firebaseio.com/v0/item/8863.json
	//			}
	//		}

}

func runCrawler() {
	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")

	databaseFilename := fmt.Sprintf("%s/hacker-news.sqlite", sqliteDataDir)

	fmt.Println("Database file", databaseFilename)
	db, err := sql.Open("sqlite3", databaseFilename)

	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	sqlQuery := "select id, gain from dataset limit 2"

	rows, err := db.Query(sqlQuery)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var gain int
		err = rows.Scan(&id, &gain)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("Got Row", id, gain)
	}
	err = rows.Err()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Successfully executed select query")
}