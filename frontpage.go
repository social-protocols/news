package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
)

// FrontPageData contains the data to populate the front page template.
type FrontPageData struct {
	Stories []Story
}

type Story struct {
	ID      int
	By      string
	Title   string
	Url     string
	Age     int
	Upvotes int
	Quality float64
	//	score   float
}

const frontPageSQL = `
	with attentionWithAge as (
		select *, datetime('now','utc')-submissionTime as age
		from attention
		order by id desc
		limit 3000
	)
	select
		id, by, title, url, age, upvotes
		, upvotes/cumulativeAttention as quality 
	from attentionWithAge join stories using(id)
	order by 
		upvotes
			/ ( cumulativeAttention * (age * age) )
	    desc
	limit 90;
`

func frontpageHandler(db *sql.DB) func(w http.ResponseWriter, r *http.Request) {

	tmpl := template.Must(template.ParseFiles("templates/index.html.tmpl"))

	statement, err := db.Prepare(frontPageSQL) // Prepare statement.
	if err != nil {
		log.Fatal(err)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := statement.Query()
		if err != nil {
			fmt.Println("Failed to get front page")
			log.Fatal(err)
		}
		defer rows.Close()

		stories := make([]Story, 0, 90)

		for rows.Next() {
			var story Story

			err = rows.Scan(&story.ID, &story.By, &story.Title, &story.Url, &story.Age, &story.Upvotes, &story.Quality)
			if err != nil {
				fmt.Println("Failed to scan row")
				log.Fatal(err)
			}
			stories = append(stories, story)

		}
		err = rows.Err()
		if err != nil {
			log.Fatal(err)
		}

		err = tmpl.Execute(w, FrontPageData{stories})
		if err != nil {
			fmt.Println(err)
		}
	}
}
