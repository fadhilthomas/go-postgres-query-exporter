package main

import (
	"fmt"
	"github.com/fadhilthomas/go-postgres-query-exporter/config"
	"github.com/hpcloud/tail"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func checkFileName(filename string, f chan<- string) {
	for {
		newFilename := fmt.Sprintf("%v", time.Now().Format(config.Get(config.LOG_FILE)+"postgresql-2006-01-02.log"))

		// if detect new filename, send to channel
		if newFilename != filename {
			log.Info().Str("file", "main").Msg(newFilename)
			f <- newFilename
			return
		}
	}
}

func listenLog() {
	var reSelect = regexp.MustCompile("duration: ([0-9]+.[0-9]+) ms {2}statement: (select|update|delete)")
	var reError = regexp.MustCompile("(error|fatal)")
	fileNameChan := make(chan string)

	for {
		filename := fmt.Sprintf("%v", time.Now().Format(config.Get(config.LOG_FILE)+"postgresql-2006-01-02.log"))

		logTail, err := tail.TailFile(filename, tail.Config{Follow: true, ReOpen: true, MustExist: true})
		if err != nil {
			log.Error().Str("file", "main").Msg(fmt.Sprintf("no log file %s. waiting for 1 minute", filename))
			time.Sleep(time.Minute * 1)
			continue
		}

		// send goroutine for checking filename in loop
		go checkFileName(filename, fileNameChan)

		// send goroutine that stops lines read in range
		go func() {
			for {
				select {
				case name := <-fileNameChan:
					log.Info().Str("file", "main").Msg(fmt.Sprintf("received a new name for log: %s", name))
					err := logTail.Stop()
					if err != nil {
						log.Error().Stack().Str("file", "main").Msg(err.Error())
						return
					}
					return
				}
			}
		}()

		if err != nil {
			log.Error().Stack().Str("file", "main").Msg(err.Error())
		}
		for line := range logTail.Lines {
			lineLow := strings.ToLower(line.Text)
			ddlMatch := reSelect.MatchString(lineLow)

			// check if log contain ddl query
			if ddlMatch == true {
				queryCounter.WithLabelValues(reSelect.FindStringSubmatch(lineLow)[2]).Inc()
				duration, err := strconv.ParseFloat(reSelect.FindStringSubmatch(lineLow)[1], 64)
				if err != nil {
					log.Error().Stack().Str("file", "main").Msg(err.Error())
				}
				queryHistogram.WithLabelValues(reSelect.FindStringSubmatch(lineLow)[2]).Observe(duration / 1000)
				log.Debug().Str("file", "main").Msg(fmt.Sprintf("%s: %s", reSelect.FindStringSubmatch(lineLow)[1],reSelect.FindStringSubmatch(lineLow)[2]))
			}

			// check if log contain error
			errMatch := reError.MatchString(lineLow)
			if errMatch == true {
				queryCounter.WithLabelValues(reError.FindStringSubmatch(lineLow)[1]).Inc()
				log.Debug().Str("file", "main").Msg(reError.FindStringSubmatch(lineLow)[1])
			}
		}
	}
}

func setLogLevel(){
	if config.Get(config.LOG_FILE) == "debug"{
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func initMetric()  {
	http.Handle("/metrics", promhttp.Handler())
	prometheus.MustRegister(version)
	prometheus.MustRegister(queryCounter)
	prometheus.MustRegister(queryHistogram)
}

var (
	appVersion = "1.0"
	version    = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "version",
		Help: "Version information about this exporter",
		ConstLabels: map[string]string{
			"postgres_query_exporter_build_info": appVersion,
		},
	})

	queryCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "postgres",
			Name:      "query_total",
			Help:      "Postgres query total calls",
		}, []string{"query"})

	queryHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "postgres",
		Name:        "query_duration_seconds",
		Help:        "Postgres query duration in seconds",
	}, []string{"query"})
)

func main() {
	setLogLevel()
	initMetric()
	go listenLog()

	log.Info().Str("file", "main").Msg("serving requests on port 9080")
	err := http.ListenAndServe(":9080", nil)
	if err != nil {
		log.Error().Stack().Str("file", "main").Msg(err.Error())
		return
	}
}
