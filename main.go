package main

import (
	"fmt"
	"github.com/fadhilthomas/go-postgres-query-exporter/config"
	"github.com/hpcloud/tail"
	"github.com/hpcloud/tail/ratelimiter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	_ "net/http/pprof"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	rePreQuery = regexp.MustCompile(`^\d{4}-\d{2}-\d{1,2} \d{2}:\d{2}:\d{2} [a-z]{3} \[\d+]: db=(\w+),user=(?:\w+-)+\w+,app=\[\w+],client=\d+.\d+.\d+.\d+ log: {2}duration: ([0-9]+.[0-9]+) ms`)
	reError    = regexp.MustCompile(`^\d{4}-\d{2}-\d{1,2} \d{2}:\d{2}:\d{2} [a-z]{3} \[\d+]: db=(\w+),user=(?:\w+-)+\w+,app=\[\w+],client=\d+.\d+.\d+.\d+ error`)
	reQuery    = regexp.MustCompile(`(select|update|delete|insert)`)
)

func checkFileName(filename string, f chan<- string) {
	timezone, err := time.ParseDuration(config.GetStr(config.LOG_TIMEZONE))
	if err != nil {
		log.Error().Stack().Str("file", "main").Msg(err.Error())
	}
	for {
		newFilename := fmt.Sprintf("%v", time.Now().Add(timezone).Format(config.GetStr(config.LOG_FILE)+"postgresql-2006-01-02.log"))

		// if detect new filename, send to channel
		if newFilename != filename {
			f <- newFilename
			return
		}
		time.Sleep(config.GetDuration(config.LOG_INTERVAL))
	}
}

func listenLog() {
	queryMap := make(map[string]string)
	errorMap := make(map[string]string)

	fileNameChan := make(chan string)

	for {
		filename := fmt.Sprintf("%v", time.Now().Add(config.GetDuration(config.LOG_TIMEZONE)).Format(config.GetStr(config.LOG_FILE)+"postgresql-2006-01-02.log"))
		log.Debug().Str("file", "main").Msg(fmt.Sprintf("log processed: %s", filename))

		tailConfig := tail.Config{
			Follow:      true,
			ReOpen:      true,
			MustExist:   true,
			RateLimiter: ratelimiter.NewLeakyBucket(uint16(config.GetInt(config.RATE_LIMIT)), config.GetDuration(config.RATE_INTERVAL)),
		}

		logTail, err := tail.TailFile(filename, tailConfig)
		if err != nil {
			log.Error().Str("file", "main").Msg(fmt.Sprintf("no log file %s. waiting for %s minute", filename, config.GetStr(config.LOG_INTERVAL)))
			time.Sleep(config.GetDuration(config.LOG_INTERVAL))
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
					logTail.Cleanup()
					if err != nil {
						log.Error().Stack().Str("file", "main").Msg(err.Error())
						return
					}
					return
				}
			}
		}()

		for line := range logTail.Lines {
			lineLow := strings.ToLower(line.Text)

			preQueryExt := rePreQuery.FindStringSubmatch(lineLow)
			if len(preQueryExt) == 3 {
				queryMap = make(map[string]string)
				queryMap["database"] = preQueryExt[1]
				queryMap["duration"] = preQueryExt[2]
			}

			errorExt := reError.FindStringSubmatch(lineLow)
			if len(errorExt) == 2 {
				errorMap = make(map[string]string)
				errorMap["database"] = errorExt[1]
				errorMap["error"] = "error"
			}

			if len(queryMap) == 2 {
				queryExt := reQuery.FindStringSubmatch(lineLow)
				if len(queryExt) == 2 {
					queryMap["query"] = queryExt[1]
				}
			}

			if len(queryMap) == 3 {
				queryCounter.WithLabelValues(queryMap["database"], queryMap["query"]).Inc()
				durationLog, err := strconv.ParseFloat(queryMap["duration"], 64)
				if err != nil {
					log.Error().Stack().Str("file", "main").Msg(err.Error())
				}
				queryHistogram.WithLabelValues(queryMap["database"], queryMap["query"]).Observe(durationLog / 1000)
				log.Debug().Str("file", "main").Msg(fmt.Sprintf("db=%s,duration=%.2f,query=%s", queryMap["database"], durationLog, queryMap["query"]))
				queryMap = make(map[string]string)
			}
			if len(errorMap) == 2 {
				queryCounter.WithLabelValues(errorMap["database"], errorMap["error"]).Inc()
				log.Debug().Str("file", "main").Msg(fmt.Sprintf("db=%s,error=%s", errorMap["database"], errorMap["error"]))
				errorMap = make(map[string]string)
			}
		}
	}
}

func initMain() {
	config.Set(config.LOG_LEVEL, "info")
	config.Set(config.LOG_TIMEZONE, "0h")
	if config.GetStr(config.LOG_LEVEL) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	runtime.GOMAXPROCS(1)
}

func initMetric() {
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
		}, []string{"database", "query"})

	queryHistogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "postgres",
		Name:      "query_duration_seconds",
		Help:      "Postgres query duration in seconds",
	}, []string{"database", "query"})
)

func main() {
	initMain()
	initMetric()
	go listenLog()

	log.Info().Str("file", "main").Msg("serving requests on port 9080")
	err := http.ListenAndServe(":9080", nil)
	if err != nil {
		log.Error().Stack().Str("file", "main").Msg(err.Error())
		return
	}
}
