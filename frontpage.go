package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/pkg/errors"
)

type frontPageData struct {
	Stories        []story
	AverageAge     float64
	AverageQuality float64
	AverageUpvotes float64
	Ranking        string
	Params         FrontPageParams
}

func (d frontPageData) AverageAgeString() string {
	return humanize.Time(time.Unix(time.Now().Unix()-int64(d.AverageAge), 0))

}

func (d frontPageData) AverageQualityString() string {
	return fmt.Sprintf("%.2f", d.AverageQuality)
}

func (d frontPageData) AverageUpvotesString() string {
	return fmt.Sprintf("%.0f", d.AverageUpvotes)
}

func (d frontPageData) Quality() bool {
	return d.Ranking == "quality"
}

func (d frontPageData) HNTop() bool {
	return d.Ranking == "hntop"
}

func (d frontPageData) GravityString() string {
	return fmt.Sprintf("%.2f", d.Params.Gravity)
}

func (d frontPageData) PriorWeightString() string {
	return fmt.Sprintf("%.2f", d.Params.PriorWeight)
}

func (d frontPageData) OverallPriorWeightString() string {
	return fmt.Sprintf("%.2f", d.Params.OverallPriorWeight)
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
	TopRank        int32
	QNRank         int32
}

func (s story) AgeString() string {
	return humanize.Time(time.Unix(int64(s.SubmissionTime), 0))
}

func (s story) QualityString() string {
	return fmt.Sprintf("%.2f", s.Quality)
}

func (s story) HNRankString() string {

	// if s.TopRank == -1 { return "" }
	//⨂

	if s.TopRank == 0 {
		return ""
	}

	return fmt.Sprintf("%d", s.TopRank)
}

func (s story) QNRankString() string {

	// if s.QNRank == -1 { return "" }

	if s.QNRank == 0 {
		return ""
	}

	return fmt.Sprintf("%d", s.QNRank)
}

type FrontPageParams struct {
	PriorWeight        float64
	OverallPriorWeight float64
	Gravity            float64
}

func (p FrontPageParams) String() string {
	return fmt.Sprintf("%#v", p)
}

var defaultFrontPageParams = FrontPageParams{2.2956, 5.0, 1.4}
var noFrontPageParams FrontPageParams

const frontPageSQL = `
  with parameters as (select %f as priorWeight, %f as overallPriorWeight, %f as gravity),
       penalties as (
         select id, sampleTime, min(score) filter (where score > 0.1) over (partition by sampleTime order by topRank rows unbounded preceding)  / score as penaltyFactor
         from (
           select id, sampleTime, topRank,
           pow(score-1, 0.8) / pow((sampleTime - submissionTime)/3600+2, 1.8) as score
           from dataset
           where topRank is not null
         ) where score > 0
       )
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
    , topRank
    , qnRank
  from stories
  join dataset using(id)
  join parameters
  left join penalties using(id, sampleTime)
  where sampleTime = (select max(sampleTime) from dataset)
  order by 
    pow((cumulativeUpvotes + overallPriorWeight)/(cumulativeExpectedUpvotes + overallPriorWeight) * ageHours, 0.8) 
    / pow(ageHours+ 2, gravity) 
    * ifnull(penalties.penaltyFactor,1) 
    desc
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
    , topRank
    , qnRank
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

var frontPageTemplate = template.Must(template.ParseFS(resources, "templates/*"))

var statements map[string]*sql.Stmt

func (app app) generateAndCacheFrontPages() error {

	// generate quality page with default params and cache
	{
		ranking := "quality"
		b, d, err := app.generateFrontPage(ranking, defaultFrontPageParams)

		// Save our story rankings to update later in the database.
		app.logger.Debug("Generated front page", "nStories", len(d.Stories))
		if err != nil {
			return errors.Wrapf(err, "generateFrontPage for ranking '%s'", ranking)
		}

		// Cache it
		app.generatedPages[ranking] = b

		// Pull out rankings and update
		rankings := make([]int, len(d.Stories))
		for i, s := range d.Stories {
			rankings[i] = s.ID
		}

		err = app.insertQNRanks(rankings)
		if err != nil {
			return errors.Wrap(err, "insertQNRanks")
		}

	}

	// hntop has to be generated **after** insertQNRanks or we don't
	// get up-to-date QN rank dat. This seems a bit messy.
	{
		ranking := "hntop"
		b, _, err := app.generateFrontPage(ranking, defaultFrontPageParams)
		if err != nil {
			return errors.Wrapf(err, "generateFrontPage for ranking '%s'", ranking)
		}
		app.generatedPages[ranking] = b

	}

	return nil
}

func (app app) generateFrontPage(ranking string, params FrontPageParams) ([]byte, frontPageData, error) {
	d, err := app.getFrontPageData(ranking, params)
	if err != nil {
		return nil, d, errors.Wrap(err, "getFrontPageData")
	}

	b, err := app.renderFrontPage(d)
	if err != nil {
		return nil, d, errors.Wrap(err, "generateFrontPageHTML")
	}

	return b, d, nil
}

func (app app) renderFrontPage(d frontPageData) ([]byte, error) {
	var b bytes.Buffer

	zw := gzip.NewWriter(&b)
	defer zw.Close()

	if err := frontPageTemplate.ExecuteTemplate(zw, "index.html.tmpl", d); err != nil {
		return nil, errors.Wrap(err, "executing front page template")
	}

	zw.Close()

	return b.Bytes(), nil
}

func (app app) getFrontPageData(ranking string, params FrontPageParams) (frontPageData, error) {

	logger := app.logger
	ndb := app.ndb

	var sampleTime int64 = time.Now().Unix()

	logger.Info("Rendering front page", "ranking", ranking)

	stories, err := getFrontPageStories(ndb, ranking, params)
	if err != nil {
		return frontPageData{}, errors.Wrap(err, "getFrontPageStories")
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

	d := frontPageData{
		stories,
		float64(totalAgeSeconds) / float64(nStories),
		weightedAverageQuality,
		float64(totalUpvotes) / float64(nStories),
		ranking,
		params,
	}

	return d, nil
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

		var topRank sql.NullInt32
		var qnRank sql.NullInt32

		err = rows.Scan(&s.ID, &s.By, &s.Title, &s.URL, &s.SubmissionTime, &ageHours, &s.Upvotes, &s.Comments, &s.Quality, &topRank, &qnRank)

		if ranking == "quality" {
			s.TopRank = topRank.Int32
		}

		if ranking == "hntop" {
			s.QNRank = qnRank.Int32
		}

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
