package main

import (
	"database/sql"
	"log"
	"time"

	"github.com/johnwarden/hn"

	"github.com/pkg/errors"
)

type getStoriesFunc func() ([]int, error)

type ranksArray [5]int

type dataPoint struct {
	id             int
	score          int
	descendants    int
	submissionTime int64
	sampleTime     int64
	ranks          ranksArray
}

func rankToNullableInt(rank int) (result sql.NullInt32) {
	if rank == 0 {
		result = sql.NullInt32{}
	} else {
		result = sql.NullInt32{Int32: int32(rank), Valid: true}

	}
	return
}

func rankCrawler(ndb newsDatabase, client *hn.Client, logger leveledLogger) {
	ticker := time.NewTicker(60 * time.Second)
	quit := make(chan struct{})
	rankCrawlerStep(ndb, client, logger)
	for {
		select {
		case <-ticker.C:
			rankCrawlerStep(ndb, client, logger)

		case <-quit:
			ticker.Stop()
			return
		}
	}

}

func rankCrawlerStep(ndb newsDatabase, client *hn.Client, logger leveledLogger) {

	sampleTime := time.Now().Unix()

	pageTypes := map[int]string{
		0: "top",
		1: "new",
		2: "best",
		3: "ask",
		4: "show",
	}

	ranksMap := map[int]ranksArray{}

	getKeys := func(m map[int]ranksArray) []int {
		keys := make([]int, len(m))
		i := 0
		for key := range m {
			keys[i] = key
			i++
		}
		return keys
	}

	// calculate ranks
	for pageType, pageTypeString := range pageTypes {
		ids, err := client.Stories(pageTypeString)
		if err != nil {
			log.Fatal(err)
		}

		for i, id := range ids {
			var ranks ranksArray
			var ok bool

			if ranks, ok = ranksMap[id]; !ok {
				ranks = ranksArray{}
			}

			ranks[pageType] = i + 1
			ranksMap[id] = ranks

			// only take the first 90 ranks
			if i+1 >= 90 {
				break
			}
		}
	}

	uniqueStoryIds := getKeys(ranksMap)

	// get story details
	logger.Info("Getting details for stories", "num_stories", len(uniqueStoryIds))

	items, err := client.GetItems(uniqueStoryIds)
	if err != nil {
		logger.Err(errors.Wrap(err, "client.GetItems"))
	}

	logger.Info("Inserting rank data", "nitems", len(items))
	// get details for every unique story
ITEM:
	for _, item := range items {
		// Skip any items that were not fetched successfully.
		if item.ID == 0 {
			continue ITEM
		}
		storyID := item.ID
		ranks := ranksMap[storyID]

		submissionTime := int64(item.Time().Unix())
		datapoint := dataPoint{
			id:             storyID,
			score:          item.Score,
			descendants:    item.Descendants,
			submissionTime: submissionTime,
			sampleTime:     sampleTime,
			ranks:          ranks,
		}

		var deltaUpvotes int
		{
			lastSeenScore, err := ndb.selectLastSeenScore(storyID)
			if err != nil {
				logger.Err(errors.Wrap(err, "selectLastSeenScore"))
				deltaUpvotes = 0
			} else {
				deltaUpvotes = item.Score - lastSeenScore
			}
		}

		err := ndb.insertDataPoint(datapoint)
		if err != nil {
			log.Fatal(err)
		}
		err = ndb.insertOrReplaceStory(item)
		if err != nil {
			log.Fatal(err)
		}

	RANKS:
		for pageType, rank := range ranks {
			if rank == 0 {
				continue RANKS
			}
			accumulateAttention(ndb, logger, pageType, storyID, rank, sampleTime, deltaUpvotes, item.Score, item.Descendants, submissionTime)
		}

	}
	logger.Info("Successfully inserted rank data", "nitems", len(items))

}
