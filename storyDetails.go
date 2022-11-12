package main

import (
	"database/sql"
	"fmt"
	"time"

	humanize "github.com/dustin/go-humanize"
)

type Story struct {
	ID                     int
	By                     string
	Title                  string
	URL                    string
	SubmissionTime         int64
	OriginalSubmissionTime int64
	AgeApprox              int64
	Score                  int
	Comments               int
	Quality                float64
	Penalty                float64
	TopRank                sql.NullInt32
	QNRank                 sql.NullInt32
}

func (s Story) AgeString() string {
	return humanize.Time(time.Unix(int64(time.Now().Unix()-s.AgeApprox), 0))
}

func (s Story) OriginalAgeString() string {
	return humanize.Time(time.Unix(s.OriginalSubmissionTime, 0))
}

func (s Story) IsResubmitted() bool {
	return s.SubmissionTime != s.OriginalSubmissionTime
}

func (s Story) QualityString() string {
	return fmt.Sprintf("%.2f", s.Quality)
}

func (s Story) PenaltyString() string {
	return fmt.Sprintf("%.0f", s.Penalty*100)
}

func (s Story) HasPenalty() bool {
	return s.Penalty > 0.0
}
