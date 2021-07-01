package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// WeatherConfig supports grabbing weather reports for given zip codes
type WeatherConfig struct {
	Topic     string `yaml:"topic"`
	Key       string `yaml:"key"`
	Locations []struct {
		Zipcode string       `yaml:"zipcode"`
		Jobs    JobRunnerCfg `yaml:"jobs"`
	} `yaml:"locations"`
}

func initWeather(bmux BrokerMux, cfg WeatherConfig) {
	if len(cfg.Topic) <= 0 {
		return
	}

	if len(cfg.Key) <= 0 {
		log.Errorf("weather: empty key")
	}

	for _, location := range cfg.Locations {
		zipcode := location.Zipcode
		NewJobRunner("weather"+"-"+location.Zipcode, location.Jobs, func() {
			reportWeather(bmux, cfg.Topic, cfg.Key, zipcode)
		}).Run()
	}
}

type weatherData struct {
	Weather []struct {
		Description string `json:"description"`
	} `json:"weather"`
	Main struct {
		Temperature float32 `json:"temp"`
		Pressure    int32   `json:"pressure"`
		Humidity    uint32  `json:"humidity"`
	} `json:"main"`
	Wind struct {
		Speed     float32 `json:"speed"`
		Direction uint32  `json:"deg"`
	} `json:"wind"`
}

func reportWeather(bmux BrokerMux, topic string, key string, zipcode string) {
	var myClient = &http.Client{Timeout: 10 * time.Second}
	var myResp weatherData

	//
	// Read it
	//
	uri := fmt.Sprintf("http://api.openweathermap.org/data/2.5/weather?zip=%s&APPID=%s&units=imperial", zipcode, key)
	rawResp, err := myClient.Get(uri)
	if err != nil {
		log.Errorf("weather: GET error: %s", err.Error())
		return
	}
	defer rawResp.Body.Close()

	if rawResp.StatusCode != http.StatusOK {
		log.Errorf("weather: GET bad response: %d", rawResp.StatusCode)
		return
	}

	//
	// Parse it
	//
	err = json.NewDecoder(rawResp.Body).Decode(&myResp)
	if err != nil {
		log.Errorf("weather: parse error: %s", err.Error())
		return
	}

	//
	// Create the event
	//
	temp := myResp.Main.Temperature
	wind := myResp.Wind.Speed

	conditions := ""
	for _, c := range myResp.Weather {
		if len(conditions) > 0 {
			conditions = conditions + ", "
		}
		conditions = conditions + c.Description
	}

	event := fmt.Sprintf("%s is %.0f\xB0 with %s and %.0f MPH wind", zipcode, temp, conditions, wind)

	//
	// Publish it
	//
	log.Infof("weather: %s", event)
	bmux.Publish(topic, 0, false, event)
}
