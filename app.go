package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/johnwarden/hn"
	"github.com/julienschmidt/httprouter"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

func main() {
	fmt.Println("In main")

	logLevelString := os.Getenv("LOG_LEVEL")

	if logLevelString == "" {
		logLevelString = "DEBUG"
	}

	sqliteDataDir := os.Getenv("SQLITE_DATA_DIR")
	if sqliteDataDir == "" {
		panic("SQLITE_DATA_DIR not set")
	}

	db, err := openNewsDatabase(sqliteDataDir)

	if err != nil {
		log.Fatal(err)
	}

	defer db.close()

	logger := newLogger(logLevelString)

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 5 * time.Second

	{
		l := logger
		l.level = logLevelInfo
		retryClient.Logger = l // ignore debug messages from this retry client.
	}

	c := hn.NewClient(retryClient.StandardClient())

	go rankCrawler(db, c, logger)

	httpServer(db)

}

func httpServer(db newsDatabase) {

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := httprouter.New()
	router.ServeFiles("/static/*filepath", http.Dir("static"))
	router.GET("/", frontpageHandler(db))

	log.Println("listening on", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
