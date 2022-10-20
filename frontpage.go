package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"time"

	"github.com/pkg/errors"
	humanize "github.com/dustin/go-humanize"

)

type frontPageData struct {
	Stories					[]story
	AverageAge			float64
	AverageQuality	float64
	AverageUpvotes  float64
}

func (d frontPageData) AverageAgeString() string {
	return humanize.Time(time.Unix(time.Now().Unix() - int64(d.AverageAge), 0))

}

func (d frontPageData) AverageQualityString() string {
	return fmt.Sprintf("%.2f", d.AverageQuality)
}

func (d frontPageData) AverageUpvotesString() string {
	return fmt.Sprintf("%.0f", d.AverageUpvotes)
}


type story struct {
	ID             int
	By             string
	Title          string
	URL            string
	SubmissionTime int64
	Upvotes        int
	Comments       int
	Quality        float64
}

func (s story) AgeString() string {
	return humanize.Time(time.Unix(int64(s.SubmissionTime), 0))
}

func (s story) QualityString() string {
	return fmt.Sprintf("%.2f", s.Quality)
}


type FrontPageParams struct {
	PriorWeight float64 
	OverallPriorWeight float64 
	Gravity float64 
}

func (p FrontPageParams) String() string {
	return fmt.Sprintf("%#v", p)
}

var defaultFrontPageParams = FrontPageParams{2.2956, 5.0, 1.4}
var noFrontPageParams FrontPageParams

const frontPageSQL = `
  with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity)
  select
    id
    , by
    , title
    , url
    , submissionTime
    , cast(unixepoch()-submissionTime as real)/3600 as ageHours
    , score
    , descendants
    , (cumulativeUpvotes + priorWeight)/(cumulativeExpectedUpvotes + priorWeight) as quality 
  from stories
  join dataset using(id)
  join parameters
  where sampleTime = (select max(sampleTime) from dataset)
  order by pow((cumulativeUpvotes + overallPriorWeight)/(cumulativeExpectedUpvotes + overallPriorWeight) * ageHours, 0.8) / pow(ageHours+ 2, gravity) desc
  limit 90;
`

const hnTopPageSQL = `
  with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity)
  select
    id
    , by
    , title
    , url
    , submissionTime
    , cast(unixepoch()-submissionTime as real)/3600 as age
    , score
    , descendants
    , (cumulativeUpvotes + priorWeight)/(cumulativeExpectedUpvotes+2.2956) as quality 
  from stories
  join dataset using(id)
  join  parameters
  where sampleTime = (select max(sampleTime) from dataset) and toprank is not null
  order by toprank asc
  limit 90;
`

/* The constant k comes from bayesian-average-quality.R (in the hacker-news-data repo).

   Bayesian Average Quality Formula

   	quality ≈ (upvotes+k)/(cumulativeExpectedUpvotes+k)

   Then add age. We want the age penalty to mimic the original HN formula:

	   pow(upvotes, 0.8) / pow(ageHours + 2, 1.8)

	The age penalty actually serves two purposes: 1) a proxy for attention and 2) to make
	sure stories cycle through the home page.

	But if we find that cumulativeExpectedUpvotes roughly equals ageHours^f, then an
	age penalty is already "built in" to our formula. But our guess is that
	f is something like 0.6, so we need to add an addition penalty of:


		(ageHours+2)^(1.8-f)

	So the ranking formula is:

   	quality ≈ (upvotes+k)/(cumulativeExpectedUpvotes+k)/(ageHours+2)^(1.8-f)

*/

//go:embed templates/*
var resources embed.FS

var t = template.Must(template.ParseFS(resources, "templates/*"))

var pages map[string][]byte
var statements map[string]*sql.Stmt

func renderFrontPages(ndb newsDatabase, logger leveledLogger) error {
	rankings := []string{"quality", "hntop"}

	for _, ranking := range rankings {
		bytes, err := renderFrontPage(ndb, logger, ranking, defaultFrontPageParams)
		if err != nil {
			return errors.Wrap(err, fmt.Sprintf("render %s page", ranking))
		}
		pages[ranking] = bytes
	}

	return nil
}

func renderFrontPage(ndb newsDatabase, logger leveledLogger, ranking string, params FrontPageParams) ([]byte, error) {

	var sampleTime int64 = time.Now().Unix()

	logger.Info("Rendering front page", "ranking", ranking)


	stories, err := getFrontPageStories(ndb, ranking, params)
	if err != nil {
		return nil, errors.Wrap(err, "getFrontPageStories")
	}

	nStories := len(stories)

	var totalAgeSeconds int64
	var weightedAverageQuality float64
	var totalUpvotes int
	for zeroBasedRank, s := range stories {
		totalAgeSeconds += (sampleTime - s.SubmissionTime)
		weightedAverageQuality += expectedUpvoteShare(0, zeroBasedRank+1) * s.Quality
		totalUpvotes += s.Upvotes
	}

	var b bytes.Buffer

	zw := gzip.NewWriter(&b)
	defer zw.Close()

	d := frontPageData{
		stories,
		float64(totalAgeSeconds) / float64(nStories),
		weightedAverageQuality,
		float64(totalUpvotes) / float64(nStories),

	}
	if err = t.ExecuteTemplate(zw, "index.html.tmpl", d); err != nil {
		return nil, errors.Wrap(err, "executing front page template")
	}

	if pages == nil {
		pages = make(map[string][]byte)
	}
	zw.Close()

	return b.Bytes(), nil
}

func getFrontPageStories(ndb newsDatabase, ranking string, params FrontPageParams) (stories []story, err error) {

	gravity := params.Gravity
	overallPriorWeight := params.OverallPriorWeight
	priorWeight := params.PriorWeight

	if statements == nil {
		statements = make(map[string]*sql.Stmt)
	}

	var s *sql.Stmt

	// Prepare statement if it hasn't already been prepared or if we are using
	// custom parameters
	if statements[ranking] == nil || params != defaultFrontPageParams {

		var sql string
		if ranking == "quality" {
			sql = fmt.Sprintf(frontPageSQL, priorWeight, overallPriorWeight, gravity)
		} else if ranking == "hntop" {
			sql = fmt.Sprintf(hnTopPageSQL, priorWeight, overallPriorWeight, gravity)
		}

		s, err = ndb.db.Prepare(sql)
		if err != nil {
			return stories, errors.Wrap(err, "preparing front page SQL")
		}

		if params != defaultFrontPageParams {
			statements[ranking] = s
		}
	} else {
		s = statements[ranking]
	}

	rows, err := s.Query()
	if err != nil {
		return stories, errors.Wrap(err, "executing front page SQL")
	}
	defer rows.Close()

	for rows.Next() {

		var s story

		var ageHours float64
		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &ageHours, &s.Upvotes, &s.Comments, &s.Quality)

		if err != nil {
			return stories, errors.Wrap(err, "Scanning row")
		}
		stories = append(stories, s)
	}

	err = rows.Err()
	if err != nil {
		return stories, err
	}

	return stories, nil

}

